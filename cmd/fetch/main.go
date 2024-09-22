package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/tomaszjudym/goger"
	"golang.org/x/sync/errgroup"
)

type Repo interface {
	CreateGames([]goger.ProductRepo) (int, error)
	CreateReviews([]goger.ReviewRepo) (int, error)
	GameReviewsCount(id string) (int, error)
	SaveNow() error
	CreateRun(r goger.RunRepo) error
}

type fetcher struct {
	repo   Repo
	logger *slog.Logger
}

type reviewer struct {
	repo   Repo
	logger *slog.Logger
}

func newReviewer(r Repo, l *slog.Logger) *reviewer {
	return &reviewer{repo: r, logger: l}
}

func main() {
	var (
		db  *goger.GamesRepo
		err error
	)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	for {
		db, err = goger.NewRepo(logger)
		if err == nil {
			break
		}
		log.Fatalf("failed to create repo: %v, retrying in 10s...", err)
		time.Sleep(10 * time.Second)
	}

	defer func() {
		if err = db.SaveNow(); err != nil {
			log.Fatalf("Failed to save run ts: %v", err)
		}
	}()

	f := fetcher{repo: db, logger: logger}
	// TODO: Drop ticker and just make single run.
	// Schedule it with cron
	ticker := time.NewTicker(20 * time.Minute)
	defer ticker.Stop()
	for ; true; <-ticker.C {
		logger.Info("Starting fetch", "time", time.Now())
		if err = f.run(); err != nil {
			f.logger.Error("Failed to fetch all games", "error", err)
		}
	}
}

func (f *fetcher) run() error {
	// TODO: Make configurable
	const pageSize = 110
	fetchedInReq := 110
	page := 1
	total := 0
	start := time.Now()
	// TODO: Config 3
	var group errgroup.Group
	group.SetLimit(3)
	// As long number of fetched games is equal to pageSize,
	// there still can be more games to fetch.
	for ; fetchedInReq == pageSize; page++ {
		// Fetch batch of games from GOG API
		resp, err := fetchGames(page, pageSize)
		if err != nil {
			return fmt.Errorf("failed to get page: %d with: %d games: %w", page, pageSize, err)
		}
		// Track how many were fetched
		fetchedInReq = len(resp.Products)
		total += fetchedInReq
		// Save them to db
		pr, err := resp.Products.ToRepo(time.Now())
		if err != nil {
			return fmt.Errorf("failed to convert response products to repo: %w", err)
		}
		if _, err = f.repo.CreateGames(pr); err != nil {
			return fmt.Errorf("failed to insert batch of: %d games: %w", len(pr), err)
		}
		// TODO: Metrics
		// For every game get reviews of this game
		for gameID, title := range resp.GameIDsToTitles() {
			inGameID := gameID
			inTitle := title
			group.Go(func() error {
				return newReviewer(f.repo, f.logger).download(inTitle, inGameID)
			})
		}
		// TODO: Invalidate cache
	}

	if err := group.Wait(); err != nil {
		return fmt.Errorf("failed to download reviews on games page: %d: %w",
			page, err)
	}
	// Save to db when last run finished.
	end := time.Now()
	if err := f.repo.CreateRun(goger.RunRepo{
		StartTs: start,
		EndTs:   end,
		Games:   total,
		Pages:   page,
	}); err != nil {
		f.logger.Error("Failed to create run", "error", err)
	}
	f.logger.Info("Run finished", "Took", time.Since(start),
		"pages", page, "games", total)
	return nil
}

func fetchGames(page, count int) (goger.CatalogResp, error) {
	// After trial and error limit > 2000 gives 500 HTTP error code
	// Update: with 1500 also can give error. Need to add backoff
	// with less and less number of records in request
	url := fmt.Sprintf(`https://catalog.gog.com/v1/catalog?limit=%d&page=%d`,
		count, page)
	response, err := http.Get(url)
	if err != nil {
		return goger.CatalogResp{}, fmt.Errorf("failed to get games: %w", err)
	}
	defer response.Body.Close()
	// Incremental rollback. GOG seemed not happy with big queries and now
	// request for 1000 games fails with 502 error. Try to cut down this number in future.
	retryCount := 0
	for response.StatusCode == http.StatusBadGateway {
		retryCount++
		if count == 1 {
			return goger.CatalogResp{}, fmt.Errorf("request failed with status code: %d after %d retries: %w",
				response.StatusCode, retryCount, err)
		}
		time.Sleep(5 * time.Second)
		count /= 2
		url = fmt.Sprintf(`https://catalog.gog.com/v1/catalog?limit=%d&page=%d`, count, page)
		log.Printf("Retrying #%d with count %d: %s", retryCount, count, url)
		response, err = http.Get(url)
		if err != nil {
			return goger.CatalogResp{}, fmt.Errorf("failed to get games after %d retries, error: %w", retryCount, err)
		}
	}
	if response.StatusCode != http.StatusOK {
		return goger.CatalogResp{}, fmt.Errorf("request failed with status code: %d",
			response.StatusCode)
	}

	var resp goger.CatalogResp
	if err = json.NewDecoder(response.Body).Decode(&resp); err != nil {
		return goger.CatalogResp{}, fmt.Errorf("failed to decode response: %w", err)
	}
	return resp, nil
}

// TODO: Looks like it's hanging or at least not downloading reviews.
// Deadlock? Channels stuck? Add context with timeout.
func (r *reviewer) download(title, gameID string) error {
	// Every response contains total review count.
	// Get single review to figure out how many reviews
	// there are for this game in total.
	var (
		group  errgroup.Group
		inDB   int
		onPage int
		err    error
	)
	// TODO: Add context for handling timeouts
	group.Go(func() error {
		var dbErr error
		inDB, dbErr = r.reviewsCountInDB(gameID)
		return dbErr
	})
	group.Go(func() error {
		var pgErr error
		onPage, pgErr = r.reviewsCountOnPage(gameID)
		return pgErr
	})
	if err = group.Wait(); err != nil {
		return fmt.Errorf("failed to get review count: %w", err)
	}

	// Nothing to download or already have everything downloaded
	if onPage == 0 || inDB == onPage {
		return nil
	}

	l := r.logger.With("gameID", gameID, "title", title)
	l.Debug(fmt.Sprintf("Downloading %s", title), "inDB", inDB, "onPage",
		onPage, "missing", onPage-inDB)

	start := time.Now()
	// Keep downloading reviews until no new reviews are returned.
	// Context will be cancelled after 10min of no progress.
	const pageSize = 150
	reviewsChan := make(chan goger.Reviews, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// This routine fetches reviews for this game from web page.
	group.Go(func() error {
		defer close(reviewsChan)
		for {
			select {
			case <-ctx.Done():
				return errors.New("context cancelled after 10 min of no progress")
			default:
				page := int(math.Ceil(float64(inDB)/float64(pageSize))) + 1
				l.Debug("fetching page", "page", page)
				resp, err := r.fetchReviews(ctx, gameID, page, pageSize)
				if err != nil {
					return fmt.Errorf("failed to fetch reviews for: %s id: %s: %w",
						title, gameID, err)
				}
				if len(resp.Embedded.Reviews) == 0 {
					return nil
				}
				inDB += len(resp.Embedded.Reviews)
				l.Debug("sending to channel page", "page", page, "got", inDB, "of", onPage)
				reviewsChan <- resp.Embedded.Reviews
			}
		}
	})
	// This routine saves them to db.
	group.Go(func() error {
		for reviews := range reviewsChan {
			_, err = r.repo.CreateReviews(reviews.ToRepo(time.Now()))
			if err != nil {
				// TODO: Kill also first routine when this error occurs
				return fmt.Errorf("failed to insert %d reviews of: %s to db: %w",
					len(reviews), title, err)
			}
			l.Debug("page of written")
		}
		return nil
	})
	// Wait for both routines to finish and check for errors.
	if err = group.Wait(); err != nil {
		return fmt.Errorf("failed to download reviews: %w", err)
	}
	l.Info("downloaded", "revsCount", onPage, "took", time.Since(start))

	return nil
}

func (r *reviewer) reviewsCountOnPage(gameID string) (int, error) {
	resp, err := r.fetchReviews(context.Background(), gameID, 1, 1)
	if err != nil {
		return -1, fmt.Errorf("failed to fetch single review "+
			"for : %s: %w", gameID, err)
	}
	return resp.ReviewCount, nil
}

func (r *reviewer) reviewsCountInDB(gameID string) (int, error) {
	inDB, err := r.repo.GameReviewsCount(gameID)
	if err != nil {
		return -1, fmt.Errorf("failed to count reviews in db: %w", err)
	}
	return inDB, nil
}

// fetchReviews returns up to limit of reviews for game with gameID on page.
// 2nd value is a number of failed requests before success.
// 3rd is how long it took
func (r *reviewer) fetchReviews(ctx context.Context, gameID string, page,
	limit int) (goger.ReviewsResp, error) {
	url := fmt.Sprintf(
		`https://reviews.gog.com/v1/products/%s/reviews?page=%d&limit=%d`,
		gameID, page, limit)

	var (
		data goger.ReviewsResp
		res  *http.Response
		err  error
	)

	backoff := time.Second
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return goger.ReviewsResp{}, fmt.Errorf("failed to create get "+
				"request to: %s, err: %w", url, err)
		}

		res, err = http.DefaultClient.Do(req)
		if err == nil && res.StatusCode == http.StatusOK {
			break
		}

		if pageSize := limit / 2; pageSize >= 10 {
			limit = pageSize
		}

		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return goger.ReviewsResp{}, fmt.Errorf("failed to read page: %d resp body: %w",
			page, err)
	}
	if err = json.Unmarshal(b, &data); err != nil {
		return goger.ReviewsResp{}, fmt.Errorf("failed to unmarshal page: %d, body: %s, %w",
			page, string(b), err)
	}
	return data, nil
}

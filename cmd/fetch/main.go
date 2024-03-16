package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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

type reviewer struct {
	repo Repo
}

func newReviewer(r Repo) *reviewer {
	return &reviewer{repo: r}
}

func main() {
	var (
		db  *goger.GamesRepo
		err error
	)
	for {
		db, err = goger.NewRepo()
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

	ticker := time.NewTicker(20 * time.Minute)
	defer ticker.Stop()
	for ; true; <-ticker.C {
		if err = run(db); err != nil {
			log.Printf("[ERR]: Failed to fetch all games: %v", err)
		}
	}
}

func run(db Repo) error {
	// TODO: Make configurable
	const pageSize = 110
	productsCount := 110
	page := 1
	total := 0
	start := time.Now()
	for ; productsCount == pageSize; page++ {
		// Fetch batch of games from GOG API
		resp, err := fetchGames(page, pageSize)
		if err != nil {
			return fmt.Errorf("failed to get page: %d with: %d games: %w", page, pageSize, err)
		}
		// Track how many were fetched
		productsCount = len(resp.Products)
		total += productsCount
		// Save them to db
		pr, err := resp.Products.ToRepo(time.Now())
		if err != nil {
			return fmt.Errorf("failed to convert response products to repo: %w", err)
		}
		if _, err = db.CreateGames(pr); err != nil {
			return fmt.Errorf("failed to insert batch of: %d games: %w", len(pr), err)
		}
		// TODO: Metrics
		// For every game get reviews of this game
		if err = downloadReviews(db, resp.GameIDsToTitles()); err != nil {
			return fmt.Errorf("failed to download reviews on games page: %d: %w",
				page, err)
		}
	}

	end := time.Now()
	if err := db.CreateRun(goger.RunRepo{
		StartTs: start,
		EndTs:   end,
		Games:   total,
		Pages:   page,
	}); err != nil {
		log.Printf("Failed to create run: %v", err)
	}
	log.Printf("Total pages: %d\n"+
		"Total games: %d\n"+
		"Took: %v\n", page, total, time.Since(start))
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
	// Incremental rollback. GOG folks seemed not happy with big queries and now
	// request for 1000 games fails with 502 error. Try to cut down this number in future
	// until it works.
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

	// Save for debug after adding logging lib
	// log.Printf("Fetched: %d games on page: %d took: %v\n", len(resp.Products), page, took)
	return resp, nil
}

func downloadReviews(db Repo, gameIDsToTitles map[string]string) error {
	var group errgroup.Group
	// TODO: Config
	group.SetLimit(3)
	for gameID, title := range gameIDsToTitles {
		inGameID := gameID
		inTitle := title
		group.Go(func() error {
			return newReviewer(db).download(inTitle, inGameID)
		})
	}
	return group.Wait()
}

func (r *reviewer) download(title, gameID string) error {
	// Every response contains total review count.
	// Get single review to figure out how many reviews
	// there are for this game in total.
	start := time.Now()
	resp, _, _, err := r.fetchReviews(context.Background(), gameID, 1, 1)
	if err != nil {
		return fmt.Errorf("failed to fetch single review "+
			"for : %s: %w", gameID, err)
	}
	inPage := resp.ReviewCount
	// Check how many reviews are in db.
	inDB, err := r.repo.GameReviewsCount(gameID)
	if err != nil {
		return fmt.Errorf("failed to count reviews in db: %w", err)
	}
	// Nothing to download or already have everything downloaded
	if inPage == 0 || inDB >= inPage {
		return nil
	}

	log.Printf("%s has: %d reviews in db, on page: %d fetched in: %v, %d missing",
		title, inDB, inPage, time.Since(start), inPage-inDB)

	// Download all reviews from page and insert them into database.
	// Calculate how many new reviews we have in db and repeat until we have none.
	// It'll mean that reviews from fetched page and next pages already are in db.
	reviewsChan := make(chan goger.Reviews, 1)
	errs := make(chan error, 2)
	doneChan := make(chan struct{}, 1)
	pageSize := 150
	totalPages := inPage / pageSize
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer close(reviewsChan)
		for i := 1; i <= totalPages; i++ {
			select {
			case <-doneChan:
				errs <- nil
				return
			default:
				resp, _, _, err := r.fetchReviews(ctx, gameID, i, pageSize)
				if err != nil {
					errs <- fmt.Errorf("failed to fetch reviews for: %s id: %s: %w",
						title, gameID, err)
					break
				}
				reviewsChan <- resp.Embedded.Reviews
			}
		}
	}()

	go func() {
		for reviews := range reviewsChan {
			created, err := r.repo.CreateReviews(reviews.ToRepo(time.Now()))
			if err != nil {
				errs <- fmt.Errorf("failed to insert %d reviews of: %s to db: %w",
					len(reviews), title, err)
				return
			}
			if created == 0 {
				doneChan <- struct{}{}
				break
			}
		}
		errs <- nil
	}()

	err1, err2 := <-errs, <-errs
	for _, err := range []error{err1, err2} {
		if err != nil {
			return fmt.Errorf("failed to download reviews: %v", err)
		}
	}
	log.Printf("%s done in: %v", title, time.Since(start))

	return nil
}

// fetchReviews returns up to limit of reviews for game with gameID on page.
// 2nd value is a number of failed requests before success.
// 3rd is how long it took
func (r *reviewer) fetchReviews(ctx context.Context, gameID string, page,
	limit int) (goger.ReviewsResp, time.Duration, int, error) {
	url := fmt.Sprintf(
		`https://reviews.gog.com/v1/products/%s/reviews?page=%d&&limit=%d`,
		gameID, page, limit)

	start := time.Now()
	resp, failures, err := getWithBackoff(ctx, url, 9999)
	if err != nil {
		return goger.ReviewsResp{}, -1, -1, fmt.Errorf("failed to get: %s: %w", url, err)
	}
	took := time.Since(start)
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return goger.ReviewsResp{}, -1, -1, fmt.Errorf("failed to read page: %d resp body: %w",
			page, err)
	}

	var data goger.ReviewsResp
	if err = json.Unmarshal(b, &data); err != nil {
		return goger.ReviewsResp{}, -1, -1, fmt.Errorf("failed to unmarshal page: %d, body: %s, %w",
			page, string(b), err)
	}
	return data, took, failures, nil
}

func getWithBackoff(ctx context.Context, url string,
	maxRetries int) (*http.Response, int, error) {

	var (
		res *http.Response
		err error
	)
	i := 1
	for ; i < maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, -1, fmt.Errorf("failed to create get request to: %s: %w",
				url, err)
		}

		start := time.Now()
		res, err = http.DefaultClient.Do(req)
		if err == nil && res.StatusCode == http.StatusOK {
			break
		}
		took := time.Since(start)
		// Incremental backoff
		sleep := time.Duration(i) * 5 * time.Second
		code := -1
		if res != nil {
			code = res.StatusCode
		}
		log.Printf("%s attempt %d failed in: %v: msg: %s code: %d err: %v "+
			"Retrying in %v...", url, i, took, http.StatusText(code),
			code, err, sleep)
		time.Sleep(sleep)
	}

	return res, i - 1, err
}

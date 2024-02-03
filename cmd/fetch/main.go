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
	CreateGames([]goger.ProductRepo) error
	CreateReviews([]goger.ReviewRepo) error
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
	const pageSize = 1000
	productsCount := 1000
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
		if db.CreateGames(pr); err != nil {
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
	start := time.Now()
	response, err := http.Get(url)
	if err != nil {
		return goger.CatalogResp{}, fmt.Errorf("failed to get games: %w", err)
	}
	took := time.Since(start)
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return goger.CatalogResp{}, fmt.Errorf("request failed with status code: %d",
			response.StatusCode)
	}

	var resp goger.CatalogResp
	if err = json.NewDecoder(response.Body).Decode(&resp); err != nil {
		return goger.CatalogResp{}, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("Fetched: %d games on page: %d took: %v\n", len(resp.Products), page, took)
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
	start := time.Now()
	// Every response contains total review count.
	// Get single review to figure out how much reviews
	// there are for this game in total.
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
	// Amount of reviews in DB will be skipped in download,
	downloadedCount, err := r.downloadPage(gameID, title, inDB, inPage)
	if err != nil {
		return fmt.Errorf("failed to download: %s reviews: %w", title, err)
	}

	newInDB, err := r.repo.GameReviewsCount(gameID)
	if err != nil {
		return fmt.Errorf("failed to count: %s reviews: %w", title, err)
	}

	if inDB != newInDB {
		log.Printf("downloaded: %d reviews, %s's review count in DB "+
			"changed from: %d to: %d in: %v",
			downloadedCount, title, inDB, newInDB, time.Since(start))
	}

	return nil
}

func (r *reviewer) downloadPage(gameID, title string, skip, total int) (int, error) {
	// TODO: Config
	var pageSize = 300
	// Reviews are present on gog page in chrono order.
	// Donwload from last page.
	totalPages, remainder := total/pageSize, total%pageSize
	skipPages := skip / pageSize
	startPage := totalPages - skipPages
	if remainder != 0 {
		startPage += 1
	}

	downloaded := 0
	totalTime := time.Duration(0)
	totalFails := 0
	l := pageSize
	// If there's less than page missing, just first page
	// can be downloaded with less records than page size.
	missing := total - skip
	if missing < pageSize {
		startPage = 1
		pageSize = missing
	}

	// Fetch reviews and insert them in separate routines to not block
	reviewsChan := make(chan goger.Reviews, 1)
	errs := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for i := startPage; i > 0; i-- {
			resp, took, fails, err := r.fetchReviews(ctx, gameID, i, pageSize)
			if err != nil {
				errs <- fmt.Errorf("failed to fetch reviews for: %s id: %s: %w",
					title, gameID, err)
				break
			}
			totalTime += took
			totalFails += fails

			reviews := resp.Embedded.Reviews
			l = len(reviews)
			log.Printf("Fetched: %d/%d reviews "+
				"from page: %d/%d of: %s in: %v with: %d failures",
				len(reviews), resp.ReviewCount, i, resp.Pages, title, took, fails)
			downloaded += l
			reviewsChan <- reviews
			// Why here was i++?
		}
		errs <- nil
		close(reviewsChan)
	}()
	go func() {
		for reviews := range reviewsChan {
			if err := r.repo.CreateReviews(reviews.ToRepo(time.Now())); err != nil {
				errs <- fmt.Errorf("failed to insert %d reviews "+
					"of: %s to db: %w", len(reviews), title, err)
			}
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			default:
			}
		}
		errs <- nil
	}()

	err1 := <-errs
	if err1 != nil {
		cancel()
	}
	err2 := <-errs
	if err2 != nil {
		cancel()
	}

	if err1 != nil || err2 != nil {
		return -1, fmt.Errorf("failed to download reviews err1: %v, err2: %v", err1, err2)
	}
	return downloaded, nil
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
		return goger.ReviewsResp{}, -1, -1, fmt.Errorf("failed to read page: %d "+
			" resp body: %w", page, err)
	}

	var data goger.ReviewsResp
	if err = json.Unmarshal(b, &data); err != nil {
		return goger.ReviewsResp{}, -1, -1, fmt.Errorf("failed to unmarshal "+
			"page: %d, body: %s, %w:", page, string(b), err)
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
		if err != nil || res == nil {
			// TODO: Debug and logging levels. Config val for them.
			code := -1
			if res != nil {
				code = res.StatusCode
			}
			log.Printf("%s attempt %d failed in: %v: msg: %s code: %d err: %v "+
				"Retrying in %v...", url, i, took, http.StatusText(code),
				code, sleep, err)
		}
		time.Sleep(sleep)
	}

	return res, i - 1, err
}

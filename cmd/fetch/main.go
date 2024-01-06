package main

import (
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
}

func main() {
	db, err := goger.NewRepo()
	if err != nil {
		log.Fatalf("failed to create repo: %v", err)
	}

	defer func() {
		if err = db.SaveNow(); err != nil {
			log.Fatalf("Failed to save run ts: %v", err)
		}
	}()

	if err = run(db); err != nil {
		log.Fatalf("Failed to fetch all games: %v", err)
	}
}

func run(db Repo) error {
	const pageSize = 1000
	productsCount := 1000
	page := 1
	total := 0
	for ; productsCount == pageSize; page++ {
		// Get batch of games
		resp, err := fetchGames(page, pageSize)
		if err != nil {
			return fmt.Errorf("failed to get page: %d with: %d games: %w", page, pageSize, err)
		}

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
		err = downloadReviews(db, resp.GameIDsToTitles())
		if err != nil {
			return fmt.Errorf("failed to download reviews on games page: %d: %w",
				page, err)
		}
	}

	log.Printf("Total pages: %d\n"+
		"Total games: %d\n", page, total)

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
	group.SetLimit(3)
	for gameID, title := range gameIDsToTitles {
		inGameID := gameID
		inTitle := title
		group.Go(func() error {
			return reviewsWorker(db, inTitle, inGameID)
		})
	}
	return nil
}

func reviewsWorker(db Repo, title, gameID string) error {
	start := time.Now()
	// Every response contains total review count.
	// Get smallest response to get this count.
	resp, _, _, err := fetchReviews(gameID, 1, 1)
	if err != nil {
		return fmt.Errorf("failed to fetch single review "+
			"for : %s: %w", gameID, err)
	}
	inPage := resp.ReviewCount
	// Check how many reviews are in db.
	inDB, err := db.GameReviewsCount(gameID)
	if err != nil {
		return fmt.Errorf("failed to count reviews in db: %w", err)
	}
	// Nothing to download or already have everything downloaded
	if inPage == 0 || inDB >= inPage {
		return nil
	}
	// Amount of reviews in DB will be skipped in download,
	log.Printf("%s has: %d on page: %d in DB - %d to download",
		title, inPage, inDB, inPage-inDB)
	err = downloadGameReviews(db, gameID, title, inDB, inPage)
	if err != nil {
		return fmt.Errorf("failed to download: %s reviews: %w", title, err)
	}

	newInDB, err := db.GameReviewsCount(gameID)
	if err != nil {
		return fmt.Errorf("failed to count: %s reviews: %w", title, err)
	}
	log.Printf("%s from %d to %d in DB in %v", title, inDB, newInDB, time.Since(start))
	return nil
}

func reviewsState(repo Repo, gameID string) (inDB, onPage int, err error) {
	// Just single record. Every page contains pages and all records count.
	resp, _, _, err := fetchReviews(gameID, 1, 1)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to fetch single review "+
			"for : %s: %w", gameID, err)
	}

	// Check how many reviews are in DB vs how many are on gog page.
	inDB, err = repo.GameReviewsCount(gameID)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to count reviews in db: %w", err)
	}

	return inDB, resp.ReviewCount, nil
}

func downloadGameReviews(repo Repo, gameID, title string, skip, total int) error {
	// TODO: Make configurable
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
	for i := startPage; i > 0; i-- {
		resp, took, fails, err := fetchReviews(gameID, i, pageSize)
		if err != nil {
			return fmt.Errorf("failed to fetch reviews for: %s: %s: %w",
				gameID, title, err)
		}
		totalTime += took
		totalFails += fails

		reviews := resp.Embedded.Reviews
		l = len(reviews)
		log.Printf("Fetched: %d/%d reviews "+
			"from page: %d/%d of: %s in: %v with %d failures",
			len(reviews), resp.ReviewCount, i, resp.Pages, title, took, fails)
		downloaded += l

		if err := repo.CreateReviews(reviews.ToRepo(time.Now())); err != nil {
			return fmt.Errorf("failed to insert page: %d/%d of: %d reviews "+
				"of: %s to db: %w", i, resp.Pages, len(reviews), title, err)
		}
		i++
	}
	log.Printf("Downloaded in total: %d reviews for: %s in: %v with %d fails",
		downloaded, title, totalTime, totalFails)
	return nil
}

// fetchReviews returns up to limit of reviews for game with gameID on page.
// 2nd value is a number of failed requests before success.
// 3rd is how long it took
func fetchReviews(gameID string, page, limit int) (goger.ReviewsResp, time.Duration, int, error) {
	url := fmt.Sprintf(
		`https://reviews.gog.com/v1/products/%s/reviews?page=%d&&limit=%d`,
		gameID, page, limit)

	start := time.Now()
	resp, failures, err := getWithBackoff(url, 9999)
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
		return goger.ReviewsResp{}, -1, -1, fmt.Errorf("failed to unmarshal page: %d, body: %s, %w:",
			page, string(b), err)
	}
	return data, took, failures, nil
}

func getWithBackoff(url string, maxRetries int) (*http.Response, int, error) {
	var (
		resp *http.Response
		err  error
	)
	i := 1
	for ; i < maxRetries; i++ {
		//start := time.Now()
		resp, err = http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		//took := time.Since(start)
		// Incremental backoff
		sleep := time.Duration(i) * 5 * time.Second
		if err != nil || resp == nil {
			// TODO: Debug and logging levels. Config val for them.
			/*log.Printf("%s attempt %d failed in: %v: msg: %s code: %d. "+
			"Retrying in %v...", url, i, took, http.StatusText(resp.StatusCode),
			resp.StatusCode, sleep)*/
		}
		time.Sleep(sleep)
	}

	return resp, i - 1, err
}

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

	if err = downloadAllGames(db); err != nil {
		log.Fatalf("failed to fetch all games: %v", err)
	}
}

func downloadAllGames(db goger.Repo) error {
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
		if err = insertToDB(db, resp); err != nil {
			return fmt.Errorf("failed to insert page: %d of: %d games to db: %w",
				page, productsCount, err)
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

func insertToDB(repo goger.Repo, resp goger.CatalogResp) error {
	pr, err := resp.Products.ToRepo()
	if err != nil {
		return fmt.Errorf("failed to convert response products to repo: %w", err)
	}

	if repo.CreateGames(pr); err != nil {
		return fmt.Errorf("failed to insert batch of: %d games: %w", len(pr), err)
	}
	return nil
}

func downloadReviews(db goger.Repo, gameIDsToTitles map[string]string) error {
	var group errgroup.Group
	group.SetLimit(5)
	for gameID, title := range gameIDsToTitles {
		inGameID := gameID
		inTitle := title
		group.Go(func() error {
			inDB, inPage, err := reviewsState(db, inGameID)
			if err != nil {
				return fmt.Errorf("failed to get reviews state of: %s: %w",
					inTitle, err)
			}
			// Nothing to download or already have everything downloaded
			if inPage == 0 || inDB >= inPage {
				return nil
			}
			// Specify how many reviews are in db
			// to skip their download.
			log.Printf("%s has: %d onPage and: %d in DB to skip",
				inTitle, inPage, inDB)
			err = downloadGameReviews(db, inGameID, inTitle, inDB, inPage)
			if err != nil {
				return fmt.Errorf("failed to download: %s reviews: %w", inTitle, err)
			}
			return nil
		})
	}
	return nil
}

// TODO: Instead of per game - fetch games in batches. 1k at once.
// Try also fetching count in parallel
func reviewsState(repo goger.Repo, gameID string) (inDB, onPage int, err error) {
	// Just single record. Every page contains pages and all records count.
	resp, err := fetchReviews(gameID, 1, 1)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to fetch single review "+
			"for : %s: %w", gameID, err)
	}

	// Check how many reviews are in DB vs how many are on gog page.
	inDB, err = repo.CountReviewsForGame(gameID)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to count reviews in db: %w", err)
	}

	return inDB, resp.ReviewCount, nil
}

func downloadGameReviews(repo goger.Repo, gameID, title string, skip, total int) error {
	// TODO: Make configurable
	const pageSize = 200
	// Reviews are present on gog page in chrono order.
	// Donwload from last page.
	// Got number of reviews to skip and total number of reviews.
	missingPages, remainder := total/pageSize, total%pageSize // ceil
	if remainder != 0 {
		missingPages++
	}
	// How many pages there are and how many to skip.
	missingPages -= skip / pageSize

	for i := missingPages; i > 0; i-- {
		start := time.Now()
		resp, err := fetchReviews(gameID, i, pageSize)
		if err != nil {
			return fmt.Errorf("failed to fetch reviews for: %s: %s: %w",
				gameID, title, err)
		}
		took := time.Since(start)

		reviews := resp.Embedded.Reviews
		log.Printf("Fetched: %d/%d reviews from page: %d/%d of: %s in: %v",
			len(reviews), resp.ReviewCount, i, resp.Pages, title, took)

		if err := repo.CreateReviews(reviews.ToRepo()); err != nil {
			return fmt.Errorf("failed to insert page: %d/%d of: %d reviews "+
				"of: %s to db: %w", i, resp.Pages, len(reviews), title, err)
		}
	}
	return nil
}

func fetchReviews(gameID string, page, limit int) (goger.ReviewsResp, error) {
	url := fmt.Sprintf(
		`https://reviews.gog.com/v1/products/%s/reviews?page=%d&&limit=%d`,
		gameID, page, limit)
	resp, err := getWithBackoff(url, 9999)
	if err != nil {
		// What gog is angry about?
		return goger.ReviewsResp{}, fmt.Errorf("failed to get: %s: %w", url, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return goger.ReviewsResp{}, fmt.Errorf("failed to read page: %d resp body: %w",
			page, err)
	}

	var data goger.ReviewsResp
	err = json.Unmarshal(b, &data)
	if err != nil {
		return goger.ReviewsResp{}, fmt.Errorf("failed to unmarshal page: %d, body: %s, %w:",
			page, string(b), err)
	}

	return data, nil
}

func getWithBackoff(url string, maxRetries int) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	for i := 1; i < maxRetries; i++ {
		start := time.Now()
		resp, err = http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		took := time.Since(start)
		// Incremental backoff
		sleep := time.Duration(i) * 3 * time.Second
		if err != nil {
			log.Printf("Attempt %d failed in: %v: msg: %s code: %d. "+
				"Retrying in %v...", i, took, http.StatusText(resp.StatusCode),
				resp.StatusCode, sleep)
		}
		if resp != nil {
			log.Printf("Attempt %d failed in: %v: msg: %s code: %d. "+
				"Retrying in %v...", i, took, http.StatusText(resp.StatusCode),
				resp.StatusCode, sleep)
		}
		time.Sleep(sleep)
	}

	return resp, err
}

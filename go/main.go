package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

func main() {
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Failed to connect db: %v", err)
	}

	if err = clearTables(db); err != nil {
		log.Fatalf("Failed to clear tables: %v", err)
	}

	if err = fetchAllGames(db); err != nil {
		log.Fatalf("failed to fetch all games: %v", err)
	}

	if err = saveRunTimestamp(db); err != nil {
		log.Fatalf("Failed to save run ts: %v", err)
	}
}

func connectDB() (*sqlx.DB, error) {
	dbHost := "pg"
	dbPort := "5432"
	dbUser := "goger"
	dbPassword := "goger"
	dbName := "goger"

	connectionString := fmt.Sprintf("host=%s port=%s user=%s password=%s "+
		"dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sqlx.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open pg connection: %w", err)
	}
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping pg: %w", err)
	}
	return db, nil
}

func clearTables(db *sqlx.DB) error {
	// Tables to delete records from
	tables := []string{"games", "reviews"}

	// Iterate over tables and delete records
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s;", table)
		_, err := db.Exec(query)
		if err != nil {
			return fmt.Errorf("query: %s failed: %w", query, err)
		}
	}

	return nil
}

func fetchAllGames(db *sqlx.DB) error {
	const pageSize = 1000
	productsCount := 1000
	page := 1
	total := 0
	for ; productsCount == pageSize; page++ {
		resp, err := fetchGames(page, pageSize)
		if err != nil {
			return fmt.Errorf("failed to get page: %d with: %d games: %w", page, pageSize, err)
		}

		if err = insertToDB(db, resp); err != nil {
			// Save file for debugging
			b, werr := json.MarshalIndent(resp, "\t", " ")
			if werr != nil {
				return fmt.Errorf("failed to marshal resp: %v after insert err: %w", werr, err)
			}
			if werr = os.WriteFile("response.json", b, 0644); werr != nil {
				return fmt.Errorf("failed to write resp file: %v after insert err: %w", werr, err)
			}
			return fmt.Errorf("failed to insert: %v", err)
		}

		productsCount = len(resp.Products)
		total += productsCount

		// Fetch reviews for games <- close into method
		gameIDsToTitles := resp.gameIDsToTitles()
		const batchSize = 50
		rvs := make([]Review, 0, batchSize)
		for gameID, title := range gameIDsToTitles {
			// Fetch reviews for specific game
			i := 1
			revsFetchStart := time.Now()
			count := batchSize
			countForGame := 0
			for ; count == batchSize; i++ {
				start := time.Now()
				reviews, err := fetchReviews(gameID, i)
				if err != nil {
					return fmt.Errorf("failed to fetch reviews for game: %s: %s: %w",
						gameID, title, err)
				}
				took := time.Since(start)

				count = len(reviews)
				rvs = append(rvs, reviews...)
				countForGame += count
				log.Printf("Fetched: %d reviews from page: %d of: %s in: %v",
					count, page, title, took)

				// Sleep a bit to not make gog angry.
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(500)+500))

				// Insert to DB
			}
			revsFetchTook := time.Since(revsFetchStart)

			log.Printf("Fetched in total: %d reviews of: %s in: %v\n",
				countForGame, title, revsFetchTook)
		}

	}

	log.Printf("Total pages: %d\n"+
		"Total records: %d\n", page, total)

	return nil
}

func fetchGames(page, count int) (CatalogResp, error) {
	// After trial and error limit > 2000 gives 500 HTTP error code
	// Update: with 1500 also can give error. Need to add backoff
	// with less and less number of records in request
	url := fmt.Sprintf(`https://catalog.gog.com/v1/catalog?limit=%d&page=%d`,
		count, page)
	start := time.Now()
	response, err := http.Get(url)
	if err != nil {
		return CatalogResp{}, fmt.Errorf("failed to get games: %w", err)
	}
	took := time.Since(start)
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return CatalogResp{}, fmt.Errorf("request failed with status code: %d",
			response.StatusCode)
	}

	var resp CatalogResp
	if err = json.NewDecoder(response.Body).Decode(&resp); err != nil {
		return CatalogResp{}, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("Fetched: %d games on page: %d took: %v\n", len(resp.Products), page, took)
	return resp, nil
}

func insertToDB(db *sqlx.DB, resp CatalogResp) error {
	pr := make([]ProductRepo, 0, len(resp.Products))
	for _, p := range resp.Products {
		repoProduct, err := p.toRepo()
		if err != nil {
			return fmt.Errorf("failed to convert: %s to repo: %w", p.Title, err)
		}
		pr = append(pr, repoProduct)
	}

	if err := insertBatch(db, pr); err != nil {
		fmt.Printf("ERROR: %v type: %T\n", err, err)
		return fmt.Errorf("failed to insert batch of: %d games: %w", len(pr), err)
	}
	return nil
}

func connect() (*sqlx.DB, error) {
	dbHost := "pg"
	dbPort := "5432"
	dbUser := "goger"
	dbPassword := "goger"
	dbName := "goger"

	connectionString := fmt.Sprintf("host=%s port=%s user=%s password=%s "+
		"dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPassword, dbName)

	return sqlx.Open("postgres", connectionString)
}

func insertBatch(db *sqlx.DB, products []ProductRepo) error {
	builder := goqu.Insert("games").Rows(products)

	sql, _, err := builder.ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build SQL: %w", err)
	}

	start := time.Now()
	res, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	took := time.Since(start)

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	log.Printf("Inserted: %d games in: %v\n", affectedRows, took)
	l := len(products)
	if affectedRows != int64(l) {
		log.Printf("WARN: Inserted: %d/%d products from batch\n",
			affectedRows, l)
	}
	return nil
}

func fetchReviews(gameID string, page int) ([]Review, error) {
	const batchSize = 1000

	url := fmt.Sprintf(
		`https://reviews.gog.com/v1/products/%s/reviews?page=%d&&limit=%d`,
		gameID, page, batchSize)
	resp, err := getWithBackoff(url, 10)
	if err != nil {
		// What gog is angry about?
		return nil, fmt.Errorf("failed to get: %s: %w", url, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read page: %d resp body: %w",
			page, err)
	}

	var data ReviewsResp
	err = json.Unmarshal(b, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal page: %d, body: %s, %w:",
			page, string(b), err)
	}

	return data.Embedded.Reviews, nil
}

func getWithBackoff(url string, maxRetries int) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	for i := 0; i < maxRetries; i++ {
		start := time.Now()
		resp, err = http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		took := time.Since(start)

		// Incremental backoff
		sleep := (time.Duration(i) * 10 * time.Second) + 30*time.Second
		fmt.Printf("Attempt %d failed in: %v: msg: %s code: %d. "+
			"Retrying in %v...\n", i, took, http.StatusText(resp.StatusCode),
			resp.StatusCode, sleep)
		time.Sleep(sleep)
	}

	return resp, err
}

func saveRunTimestamp(db *sqlx.DB) error {
	ts := time.Now().UTC().Format("2006-01-02 15:04:05.999")
	// Upsert if there's record already
	query := `
		INSERT INTO last_run (onerow_id, ts) VALUES (true, $1)
		ON CONFLICT (onerow_id) DO UPDATE SET ts = $1
	`
	_, err := db.Exec(query, ts)
	return err
}

package main

import (
	"encoding/json"
	"fmt"
	"log"
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

	if err = fetchAllGames(db); err != nil {
		log.Fatalf("failed to fetch all games: %v", err)
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
			if err != nil {
				return fmt.Errorf("failed to marshal resp: %v after insert err: %w", werr, err)
			}
			if werr = os.WriteFile("response.json", b, 0644); werr != nil {
				return fmt.Errorf("failed to write resp file: %v after insert err: %w", werr, err)
			}
			return fmt.Errorf("failed to insert: %v", err)
		}

		productsCount = len(resp.Products)
		total += productsCount
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
			return fmt.Errorf("failed to convert: %v to repo: %w", p, err)
		}
		pr = append(pr, repoProduct)
	}

	if err := insertBatch(db, pr); err != nil {
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

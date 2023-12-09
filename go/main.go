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
	"github.com/lib/pq"
)

func main() {
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Failed to connect db: %v", err)
	}

	resp, err := getGames(1, 100)
	if err != nil {
		log.Fatalf("Failed to get page: %d with: %d games: %v", 1, 5, err)
	}

	if err = insertToDB(db, resp); err != nil {
		// Save file for debugging
		b, werr := json.MarshalIndent(resp, "\t", " ")
		if err != nil {
			log.Fatalf("failed to marshal resp: %v after insert err: %v", werr, err)
		}
		if werr = os.WriteFile("response.json", b, 0644); werr != nil {
			log.Fatalf("failed to write resp file: %v after insert err: %v", werr, err)
		}
		log.Fatalf("failedto insert: %v", err)
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

func getGames(page, count int) (CatalogResp, error) {
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
	log.Printf("Request for: %d games on page: %d took: %v\n", count, page, took)
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return CatalogResp{}, fmt.Errorf("request failed with status code: %d",
			response.StatusCode)
	}

	var resp CatalogResp
	if err = json.NewDecoder(response.Body).Decode(&resp); err != nil {
		return CatalogResp{}, fmt.Errorf("failed to decode response: %w", err)
	}
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

	// Create a connection string
	connectionString := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Open a database connection
	return sqlx.Open("postgres", connectionString)
}

func insert(db *sqlx.DB, p ProductRepo) error {
	// Insert the data into the PostgreSQL table using sqlx.Named
	const query = `
		INSERT INTO games (
			id, slug, features, screenshots, 
			user_preferred_language_code, user_preferred_language_in_audio, user_preferred_language_in_text,
			release_date, store_release_date,
			product_type, title, cover_horizontal, cover_vertical, developers, publishers,
			operating_systems, price_final, price_base, price_currency, price_discount,
			product_state, genres, tags, reviews_rating
		) VALUES (
			:id, :slug, :features, :screenshots, 
			:user_preferred_language_code, :user_preferred_language_in_audio, :user_preferred_language_in_text,
			:release_date, :store_release_date,
			:product_type, :title, :cover_horizontal, :cover_vertical, :developers, :publishers,
			:operating_systems, :price_final, :price_base, :price_currency, :price_discount,
			:product_state, :genres, :tags, :reviews_rating
		)
	`

	// Create a map with named parameters
	// Create a map with named parameters
	namedParams := map[string]interface{}{
		"id":                               p.ID,
		"slug":                             p.Slug,
		"features":                         pq.Array(p.Features),
		"screenshots":                      pq.Array(p.Screenshots),
		"user_preferred_language_code":     p.UserPreferredLangCode,
		"user_preferred_language_in_audio": p.UserPreferredLangInAudio,
		"user_preferred_language_in_text":  p.UserPreferredLangInText,
		"release_date":                     p.ReleaseDate,
		"store_release_date":               p.StoreReleaseDate,
		"product_type":                     p.ProductType,
		"title":                            p.Title,
		"cover_horizontal":                 p.CoverHorizontal,
		"cover_vertical":                   p.CoverVertical,
		"developers":                       pq.Array(p.Developers),
		"publishers":                       pq.Array(p.Publishers),
		"operating_systems":                pq.Array(p.OperatingSystems),
		"price_final":                      p.PriceFinal,
		"price_base":                       p.PriceBase,
		"price_currency":                   p.PriceCurrency,
		"price_discount":                   p.PriceDiscount,
		"product_state":                    p.ProductState,
		"genres":                           pq.Array(p.Genres),
		"tags":                             pq.Array(p.Tags),
		"reviews_rating":                   p.ReviewsRating,
	}

	// Use sqlx.Named to automatically map struct fields to database columns
	res, err := db.NamedExec(query, namedParams)
	if err != nil {
		return err
	}
	i, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	fmt.Println("Insert of", p.Slug, "affected", i, "rows")
	return err
}

func insertBatch(db *sqlx.DB, products []ProductRepo) error {
	builder := goqu.Insert("games").Rows(products)

	sql, _, err := builder.ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build SQL: %w", err)
	}

	res, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	l := len(products)
	if affectedRows != int64(l) {
		log.Printf("WARN: Inserted: %d/%d products from batch\n",
			affectedRows, l)
	}
	return nil
}

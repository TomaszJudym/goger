package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func main() {
	// Get first page
	// After trial and error limit > 2000 gives 500 HTTP error code
	// Update: with 1500 also can give error. Need to add backoff
	// with less and less number of records in request
	const url = `https://catalog.gog.com/v1/catalog?limit=5&page=1`
	response, err := http.Get(url)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer response.Body.Close()

	// Check the HTTP status code.
	if response.StatusCode != http.StatusOK {
		fmt.Printf("HTTP request failed with status code: %d\n",
			response.StatusCode)
		return
	}

	var resp CatalogResp
	if err = json.NewDecoder(response.Body).Decode(&resp); err != nil {
		log.Fatal("Failed to decode response:", err)
	}

	// Access the fields in the custom type.
	fmt.Printf("Pages: %d\n", resp.Pages)
	fmt.Printf("Product Count: %d\n", resp.ProductCount)
	fmt.Printf("Products got : %d\n", len(resp.Products))

	// For debugging write response to file
	b, err := json.MarshalIndent(resp, "\t", " ")
	if err != nil {
		log.Fatal("Failed to marshal resp:", err)
	}
	if err = os.WriteFile("response.json", b, 0644); err != nil {
		log.Fatal("Failed to write response.json:", err)
	}

	db, err := connect()
	if err != nil {
		log.Fatal("Failed to connect to postgres:", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal("failed to ping postgres", err)
	}

	var rp []ProductRepo
	for _, p := range resp.Products {
		repoProduct, err := p.toRepo()
		if err != nil {
			log.Fatalf("Failed to convert %v to repo: %v\n", resp.Products[0], err)
		}
		rp = append(rp, repoProduct)
	}

	if err = insertBatch(db, rp); err != nil {
		log.Fatalf("Failed to insert: %s: %v\n", resp.Products[0].Slug, err)
	}
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
	// Create a goqu Builder
	builder := goqu.Insert("games").Rows(products)

	// Use goqu for NamedExec
	sql, args, err := builder.ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build SQL: %w", err)
	}

	fmt.Println("ARGS: ", args)

	// Use sqlx.NamedExec to execute the query
	res, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	fmt.Printf("Insert affected %d rows\n", affectedRows)
	return nil
}

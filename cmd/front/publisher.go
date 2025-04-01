package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/jmoiron/sqlx"
)

// Data Structures
type PublisherStatistics struct {
	Publisher          string                  `db:"publisher"`
	PublishedGames     int                     `db:"published_games"`
	AvgRating          float64                 `db:"avg_publisher_rating"`
	AvgPrice           float64                 `db:"avg_price"`
	TotalReviews       int                     `db:"total_reviews"`
	AvgReviewsRating   float64                 `db:"avg_reviews_rating"`
	ReleaseTrendData   []ReleaseTrendPublisher `db:"-"` // Not directly from DB
	RatingDistribution []RatingDistribution    `db:"-"` // Not directly from DB
	PriceDistribution  []PriceDistribution     `db:"-"` // Not directly from DB
	MaxReleases        float64                 `db:"-"`
	MaxRatingCount     float64                 `db:"-"`
	MaxPriceCount      float64                 `db:"-"`
}

// ReleaseTrendPublisher Data structure for the chart
type ReleaseTrendPublisher struct {
	Year      int     `db:"release_year"`
	GameCount float64 `db:"game_count"`
}

// RatingDistribution data
type RatingDistribution struct {
	Rating float64 `db:"rating"`
	Count  float64 `db:"game_count"`
}

// PriceDistribution data
type PriceDistribution struct {
	PriceRange string  `db:"price_range"`
	Count      float64 `db:"game_count"`
}

// TODO: Finish this handler
func PublisherHandler(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publisherName := r.URL.Query().Get("name")
		if publisherName == "" {
			http.Error(w, "Missing 'name' parameter", http.StatusBadRequest)
			return
		}

		publisherData, err := getPublisherData(db, publisherName)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Publisher not found", http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf("Failed to get publisher data: %v", err), http.StatusInternalServerError)
			}
			return
		}
		//Fetch all values
		publisherData.ReleaseTrendData, err = getReleaseTrendData(db, publisherName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get release trend data: %v", err), http.StatusInternalServerError)
			return
		}
		publisherData.RatingDistribution, err = getRatingDistribution(db, publisherName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get rating distribution data: %v", err), http.StatusInternalServerError)
			return
		}
		publisherData.PriceDistribution, err = getPriceDistribution(db, publisherName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get Price Distribution data: %v", err), http.StatusInternalServerError)
			return
		}
		// Initialize max values to avoid issues if the slices are empty
		publisherData.MaxReleases = 0
		publisherData.MaxRatingCount = 0
		publisherData.MaxPriceCount = 0

		for _, element := range publisherData.ReleaseTrendData {
			if element.GameCount > publisherData.MaxReleases {
				publisherData.MaxReleases = element.GameCount
			}
		}

		for _, element := range publisherData.RatingDistribution {
			if element.Count > publisherData.MaxRatingCount {
				publisherData.MaxRatingCount = element.Count
			}
		}

		for _, element := range publisherData.PriceDistribution {
			if element.Count > publisherData.MaxPriceCount {
				publisherData.MaxPriceCount = element.Count
			}
		}
		tmpl := template.New("publisher.html").Funcs(funcMap)
		tmpl, err = tmpl.ParseFiles(filepath.Join("templates", "publisher.html"))
		if err != nil {
			http.Error(w, fmt.Sprintf("Parse: %v", err), http.StatusInternalServerError)
			return
		}

		if err = tmpl.Execute(w, publisherData); err != nil {
			http.Error(w, fmt.Sprintf("Execute: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

func getPublisherData(db *sqlx.DB, publisherName string) (PublisherStatistics, error) {
	var publisherData PublisherStatistics

	err := db.Get(&publisherData, `
		SELECT
		publisher,
		published_games,
		avg_publisher_rating,
		COALESCE(ROUND(avg_genre_price::numeric, 2), 0) AS avg_price,
		COALESCE(total_reviews, 0) AS total_reviews,
		COALESCE(ROUND(avg_genre_rating::numeric, 2), 0) AS avg_reviews_rating
	FROM (
		SELECT
			unnest(publishers) AS publisher,
			COUNT(*) AS published_games,
			ROUND(AVG(reviews_rating)::numeric, 2) AS avg_publisher_rating
		FROM games
		WHERE $1 = ANY(publishers)
		GROUP BY publisher
	) AS publisher_stats
	LEFT JOIN (
		SELECT
			AVG(price_final) AS avg_genre_price,
			AVG(reviews_rating) AS avg_genre_rating,
			COUNT(*) AS total_reviews
		FROM games
		JOIN reviews ON games.id = reviews.product_id
		WHERE $1 = ANY(publishers)
	) AS price_review_stats ON TRUE;
	`, publisherName)
	if err != nil {
		return PublisherStatistics{}, err
	}

	return publisherData, nil
}

// getReleaseTrendData fetches game release trend data for a publisher.
func getReleaseTrendData(db *sqlx.DB, publisherName string) ([]ReleaseTrendPublisher, error) {
	var data []ReleaseTrendPublisher
	err := db.Select(&data, `
	SELECT
    EXTRACT(YEAR FROM release_date) AS release_year,
    COUNT(*) AS game_count
		FROM games
		WHERE $1 = ANY(publishers)
		GROUP BY EXTRACT(YEAR FROM release_date)
		ORDER BY release_year;
	`, publisherName)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// getRatingDistribution fetches the distribution of game ratings for a publisher.
func getRatingDistribution(db *sqlx.DB, publisherName string) ([]RatingDistribution, error) {
	var data []RatingDistribution
	err := db.Select(&data, `
	SELECT
		reviews_rating AS rating,
		COUNT(*) AS game_count
	FROM games
	WHERE $1 = ANY(publishers)
	GROUP BY reviews_rating
	ORDER BY reviews_rating;
	`, publisherName)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// getPriceDistribution fetches the distribution of game prices for a publisher.
func getPriceDistribution(db *sqlx.DB, publisherName string) ([]PriceDistribution, error) {
	var data []PriceDistribution
	err := db.Select(&data, `
		SELECT price_range, game_count
        FROM (
            SELECT
                CASE
                    WHEN price_final < 5 THEN 'Under $5'
                    WHEN price_final >= 5 AND price_final < 10 THEN '$5 - $10'
                    WHEN price_final >= 10 AND price_final < 15 THEN '$10 - $15'
                    WHEN price_final >= 15 AND price_final < 20 THEN '$15 - $20'
                    WHEN price_final >= 20 AND price_final < 25 THEN '$20 - $25'
                    WHEN price_final >= 25 AND price_final < 30 THEN '$25 - $30'
                    WHEN price_final >= 30 AND price_final < 35 THEN '$30 - $35'
                    WHEN price_final >= 35 AND price_final < 40 THEN '$35 - $40'
                    WHEN price_final >= 40 AND price_final < 45 THEN '$40 - $45'
                    WHEN price_final >= 45 AND price_final < 50 THEN '$45 - $50'
                    ELSE 'Over $50'
                END AS price_range,
                COUNT(*) AS game_count,
                CASE
                    WHEN price_final < 5 THEN 1
                    WHEN price_final >= 5 AND price_final < 10 THEN 2
                    WHEN price_final >= 10 AND price_final < 15 THEN 3
                    WHEN price_final >= 15 AND price_final < 20 THEN 4
                    WHEN price_final >= 20 AND price_final < 25 THEN 5
                    WHEN price_final >= 25 AND price_final < 30 THEN 6
                    WHEN price_final >= 30 AND price_final < 35 THEN 7
                    WHEN price_final >= 35 AND price_final < 40 THEN 8
                    WHEN price_final >= 40 AND price_final < 45 THEN 9
                    WHEN price_final >= 45 AND price_final < 50 THEN 10
                    ELSE 11
                END AS sort_order
            FROM games
            WHERE $1 = ANY(publishers)
            GROUP BY price_range, sort_order
        ) AS subquery
        ORDER BY sort_order;
    `, publisherName)
	if err != nil {
		return nil, err
	}
	return data, nil
}

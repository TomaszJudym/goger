package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// GameSpecificStatistics represents statistics for a single game.
type GameSpecificStatistics struct {
	ID            int            `db:"id"`
	Title         string         `db:"title"`
	Slug          string         `db:"slug"`
	ReleaseDate   timeDate       `db:"release_date"`
	PriceFinal    float64        `db:"price_final"`
	ReviewsCount  int            `db:"reviews_count"`
	ReviewsRating int            `db:"reviews_rating"`
	Genres        pq.StringArray `db:"genres"` // Changed to StringArray
	Description   string         // Added description field
	Screenshots   pq.StringArray `db:"screenshots"` // Add this line
	ReviewData    []DataPoint    // Data for the review history bar chart
	RatingData    []DataPoint    // Data for the rating history bar chart // ADDED
}

func GameHandler(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameIDStr := r.URL.Query().Get("id")
		if gameIDStr == "" {
			http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
			return
		}

		gameID, err := strconv.Atoi(gameIDStr)
		if err != nil {
			http.Error(w, "Invalid 'id' parameter", http.StatusBadRequest)
			return
		}

		var gameData GameSpecificStatistics
		err = db.Get(&gameData, `SELECT id, title, slug, release_date, price_final, reviews_count, reviews_rating, genres, screenshots FROM games WHERE id = $1`, gameID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Game not found: %v", err), http.StatusNotFound)
			return
		}
		// Get game description (if available) - you might need to adjust the query if the description is in a different table
		err = db.Get(&gameData.Description, `SELECT description FROM reviews WHERE product_id = $1 LIMIT 1`, gameID)
		if err != nil {
			gameData.Description = "No description available" // Default if no description found
		}

		//Get review data for game
		gameData.ReviewData, err = reviewDataForGame(db, gameID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get reviews data: %v", err), http.StatusInternalServerError)
			return
		}

		// Get rating data for game
		gameData.RatingData, err = ratingDataForGame(db, gameID) // ADDED
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get rating data: %v", err), http.StatusInternalServerError)
			return
		}

		// Aggregate data if we have > 100 data points
		if len(gameData.ReviewData) > 100 {
			log.Printf("Original Review Data Length: %d\n", len(gameData.ReviewData))
			// Aggregate the data to fit 100 points
			aggregatedData := make([]DataPoint, 100)
			binSize := len(gameData.ReviewData) / 100

			for i := 0; i < 100; i++ {
				startIndex := i * binSize
				endIndex := (i + 1) * binSize
				if i == 99 {
					endIndex = len(gameData.ReviewData) // Make sure to include all remaining data in the last bin
				}

				totalReviews := 0.0
				for j := startIndex; j < endIndex; j++ {
					totalReviews += gameData.ReviewData[j].Value
				}
				binLength := endIndex - startIndex
				if binLength > 0 {
					totalReviews = totalReviews / float64(binLength)
				}

				// Calculate the average date for the bin
				var totalTime time.Time
				for j := startIndex; j < endIndex; j++ {
					totalTime = totalTime.Add(gameData.ReviewData[j].Date.Sub(time.Time{}))
				}
				aggregatedData[i] = DataPoint{
					Date:  gameData.ReviewData[startIndex].Date,
					Value: totalReviews,
				}
			}
			gameData.ReviewData = aggregatedData
		}
		// Aggregate rating data if we have > 100 data points // ADDED
		if len(gameData.RatingData) > 100 {
			log.Printf("Original Rating Data Length: %d\n", len(gameData.RatingData))
			// Aggregate the data to fit 100 points
			aggregatedData := make([]DataPoint, 100)
			binSize := len(gameData.RatingData) / 100

			for i := 0; i < 100; i++ {
				startIndex := i * binSize
				endIndex := (i + 1) * binSize
				if i == 99 {
					endIndex = len(gameData.RatingData) // Make sure to include all remaining data in the last bin
				}

				totalRatingValue := 0.0
				for j := startIndex; j < endIndex; j++ {
					totalRatingValue += gameData.RatingData[j].Value
				}
				// Calculate the average rating value for the bin
				binLength := endIndex - startIndex
				if binLength > 0 {
					totalRatingValue = totalRatingValue / float64(binLength)
				}
				// Round the average rating value to 2 decimal places
				totalRatingValue = float64(int(totalRatingValue*100)) / 100

				var totalTime time.Time
				for j := startIndex; j < endIndex; j++ {
					totalTime = totalTime.Add(gameData.RatingData[j].Date.Sub(time.Time{}))
				}
				aggregatedData[i] = DataPoint{
					Date:  gameData.RatingData[startIndex].Date,
					Value: totalRatingValue,
				}
			}
			gameData.RatingData = aggregatedData
		}
		// ... (Existing code for template function map and execution) ...
		funcMap := template.FuncMap{
			"div": func(a, b float64) float64 {
				if b == 0 {
					return 0 // Handle division by zero
				}
				return a / b
			},
			"mul": func(a, b float64) float64 {
				return a * b
			},
			"float64": func(i int) float64 {
				return float64(i)
			},
			"mod": func(a, b int) int {
				return a % b
			},
		}
		tmpl := template.New("game.html").Funcs(funcMap) // Create a new template for game-specific stats
		tmpl, err = tmpl.ParseFiles("templates/game.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Parse: %v", err), http.StatusInternalServerError)
			return
		}

		if err = tmpl.Execute(w, gameData); err != nil {
			http.Error(w, fmt.Sprintf("Execute: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// ratingDataForGame queries the database and returns rating data over time. // ADDED
func ratingDataForGame(db *sqlx.DB, gameID int) ([]DataPoint, error) {
	query := `
        SELECT
            DATE_TRUNC('day', review_date) AS review_day,
            AVG(rating_value) AS avg_rating
        FROM
            reviews
        WHERE
            review_date IS NOT NULL AND product_id = $1
        GROUP BY
            review_day
        ORDER BY
            review_day;
    `
	rows, err := db.Query(query, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []DataPoint
	for rows.Next() {
		var dp DataPoint
		var avgRating float64 // Need to scan into a float64
		if err := rows.Scan(&dp.Date, &avgRating); err != nil {
			return nil, err
		}
		dp.Value = avgRating // Convert to integer for chart
		data = append(data, dp)
	}
	return data, rows.Err()
}

// reviewDataForGame queries the database and returns review data over time.
func reviewDataForGame(db *sqlx.DB, gameID int) ([]DataPoint, error) {
	query := `
                SELECT
                        DATE_TRUNC('day', review_date) AS review_day,
                        COUNT(*) AS total_reviews
                FROM
                        reviews
                WHERE
                        review_date IS NOT NULL AND product_id = $1
                GROUP BY
                        review_day
                ORDER BY
                        review_day;
        `

	rows, err := db.Query(query, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []DataPoint
	for rows.Next() {
		var dp DataPoint
		if err := rows.Scan(&dp.Date, &dp.Value); err != nil {
			return nil, err
		}
		data = append(data, dp)
	}
	return data, rows.Err()
}

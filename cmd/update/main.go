package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lib/pq"
)

func main() {
	const (
		dbHost     = "pg"
		dbPort     = "5432"
		dbUser     = "goger"
		dbPassword = "goger"
		dbName     = "goger"
	)
	connectionString := fmt.Sprintf("host=%s port=%s user=%s password=%s "+
		"dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	listenForDBChanges(connectionString)
}

type reviewInfo struct {
	ProductID      int    `json:"product_id"`
	Title          string `json:"title"`
	DescriptionLen int    `json:"description_len"`
}

func listenForDBChanges(connStr string) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	listener := pq.NewListener(connStr, 10*time.Second, time.Hour*24*365, func(event pq.ListenerEventType, err error) {
		if err != nil {
			logger.Error("Postgres listener state change", "event", event, "error", err)
		}
	})

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		logger.Error("Failed to connect to postgres", "error", err)
	}

	err = listener.Listen("games_changes")
	if err != nil {
		logger.Error("Error listening on channel 'games_changes'", "error", err)
	}

	err = listener.Listen("reviews_changes")
	if err != nil {
		logger.Error("Error listening on channel 'reviews_changes'", "error", err)
	}
	// Fetch first time to get the initial state and not log
	// that state changed from 0 to something.
	var inDBReviews, inDBGames int
	err = db.QueryRow("SELECT count(id) FROM reviews").Scan(&inDBReviews)
	if err != nil {
		logger.Error("Failed to count reviews", "error", err)
	}
	err = db.QueryRow("SELECT count(id) FROM games").Scan(&inDBGames)
	if err != nil {
		logger.Error("Failed to count games", "error", err)
	}

	reviewsCount10sAgo, gamesCount10sAgo := inDBReviews, inDBGames
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := db.QueryRow("SELECT count(id) FROM reviews").Scan(&inDBReviews)
			if err != nil {
				logger.Error("Failed to count reviews", "error", err)
			}
			err = db.QueryRow("SELECT count(id) FROM games").Scan(&inDBGames)
			if err != nil {
				logger.Error("Failed to count games", "error", err)
			}

			if inDBReviews != reviewsCount10sAgo || inDBGames != gamesCount10sAgo {
				logger.Info("counters changed",
					"reviews_count", inDBReviews, "games_count", inDBGames)
				reviewsCount10sAgo, gamesCount10sAgo = inDBReviews, inDBGames
			}
		case n := <-listener.Notify:
			switch n.Channel {
			case "games_changes":
			case "reviews_changes":
			default:
				logger.Error("Unknown channel", "channel", n.Channel)
			}
		}
	}
}

func gameTitle(db *sql.DB, reviewProductID int) (string, error) {
	const q = `SELECT games.title FROM games
		JOIN reviews ON games.id = reviews.product_id
		WHERE reviews.product_id = $1 LIMIT 1`
	rows, err := db.Query(q, reviewProductID)
	if err != nil {
		return "", fmt.Errorf("failed to query game title by review_id: %w", err)
	}

	var title string
	for rows.Next() {
		if err = rows.Scan(&title); err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}
	}
	return title, nil
}

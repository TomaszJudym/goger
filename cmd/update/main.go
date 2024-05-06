package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	"github.com/tomaszjudym/goger"
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
	ProductID   int    `json:"product_id"`
	Description string `json:"description"`
	Title       string `json:"title"`
}

func listenForDBChanges(connStr string) {
	listener := pq.NewListener(connStr, 10*time.Second, time.Hour*24*365, func(event pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("Postgres listener state change: %d, error: %v", event, err)
		}
	})

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}

	err = listener.Listen("games_changes")
	if err != nil {
		log.Fatalf("Error listening on channel 'games_changes': %v", err)
	}

	err = listener.Listen("reviews_changes")
	if err != nil {
		log.Fatalf("Error listening on channel 'reviews_changes': %v", err)
	}

	for {
		n := <-listener.Notify
		switch n.Channel {
		case "games_changes":
			var game goger.ProductRepo
			if err = json.Unmarshal([]byte(n.Extra), &game); err != nil {
				log.Printf("Failed to unmarshal games: %s, error: %v", n.Extra, err)
			}
			fmt.Printf("UPDATE OF GAME: %s\n", game.Title)
		case "reviews_changes":
			var info reviewInfo
			if err = json.Unmarshal([]byte(n.Extra), &info); err != nil {
				log.Printf("Failed to unmarshal reviews: %s, error: %v", n.Extra, err)
			}

			gameTitle, err := gameTitle(db, info.ProductID)
			if err != nil {
				log.Printf("Failed to get game title for review with product_id: %d: %v",
					info.ProductID, err)
			}
			txtLen := len(info.Description)
			fmt.Printf("UPDATE OF REVIEW game: %s, len: %d, title: %s\n",
				gameTitle, txtLen, info.Title)
		default:
			log.Printf("Unknown channel: %s", n.Channel)
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

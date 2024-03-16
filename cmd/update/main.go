package main

import (
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

func listenForDBChanges(connStr string) {
	conn := pq.NewListener(connStr, 10*time.Second, time.Hour*24*365, func(event pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("Postgres listener state change: %d, error: %v", event, err)
		}
	})

	err := conn.Listen("games_changes")
	if err != nil {
		log.Fatalf("Error listening on channel 'games_changes': %v", err)
	}

	err = conn.Listen("reviews_changes")
	if err != nil {
		log.Fatalf("Error listening on channel 'reviews_changes': %v", err)
	}

	for {
		n := <-conn.Notify
		log.Printf("Received data change notification: %s", n.Extra)
		switch n.Channel {
		case "games_changes":
			var game goger.ProductRepo
			if err = json.Unmarshal([]byte(n.Extra), &game); err != nil {
				log.Printf("Failed to unmarshal games: %s, error: %v", n.Extra, err)
			}
			b, err := json.MarshalIndent(game, "\t,", "\t")
			if err != nil {
				log.Printf("Failed to marshal indent game: %v", err)
			}
			fmt.Printf("UPDATE OF GAME:\n%s", string(b))
		case "reviews_changes":
			var review goger.ReviewRepo
			if err = json.Unmarshal([]byte(n.Extra), &review); err != nil {
				log.Printf("Failed to unmarshal reviews: %s, error: %v", n.Extra, err)
			}
			b, err := json.MarshalIndent(review, "\t,", "\t")
			if err != nil {
				log.Printf("Failed to marshal indent review: %v", err)
			}
			fmt.Printf("UPDATE OF REVIEW:\n%s", string(b))
		default:
			log.Printf("Unknown channel: %s", n.Channel)
		}
	}
}

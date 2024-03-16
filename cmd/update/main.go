package main

import (
	"fmt"
	"log"
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
	}
}

package goger

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

func ConnectDB() (*sqlx.DB, error) {
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

func CountGames(db *sqlx.DB) (int, error) {
	const query = `SELECT COUNT(id) FROM games`
	stmt, err := db.Prepare(query)
	if err != nil {
		return -1, fmt.Errorf("failed to prepare games count stmt: %w", err)
	}
	defer stmt.Close()

	rows, err := stmt.Query()
	if err != nil {
		return -1, fmt.Errorf("failed to exec games count: %w", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err = rows.Scan(&count); err != nil {
			return -1, fmt.Errorf("failed to scan games count: %w", err)
		}
	}

	return count, nil
}

package goger

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

type Repo interface {
	CreateGames([]ProductRepo) error
	CreateReviews([]ReviewRepo) error
	CountReviewsForGame(id string) (int, error)
	CountGames() (int, error)
	GamesWithRevsiewsCount(offset, limit int) ([]UIGame, error)
	SaveNow() error
}

type GamesRepo struct {
	db                 *goqu.Database
	countGames         *sql.Stmt
	countRevsForGame   *sql.Stmt
	gamesWithRevsCount *sql.Stmt
	fetchGames         *sql.Stmt
	fetchReviwes       *sql.Stmt
}

func NewRepo() (*GamesRepo, error) {
	var (
		dbHost     = "pg"
		dbPort     = "5432"
		dbUser     = "goger"
		dbPassword = "goger"
		dbName     = "goger"
	)

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

	gq := goqu.New("postgres", db)
	countR, err := gq.Prepare("SELECT COUNT(id) FROM reviews where product_id = $1")
	if err != nil {
		return nil, err
	}
	countG, err := gq.Prepare("SELECT COUNT(id) FROM games")
	if err != nil {
		return nil, err
	}
	gamesWithReviewCount, err := gq.Prepare(`
		SELECT g.id, g.title, COUNT(r.id) AS reviews_count
		FROM games g
		LEFT JOIN reviews r ON g.id = r.product_id
		GROUP BY g.id, g.title
		ORDER BY reviews_count DESC, g.id
		OFFSET $1
		LIMIT $2`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}

	return &GamesRepo{
		db:                 gq,
		countRevsForGame:   countR,
		countGames:         countG,
		gamesWithRevsCount: gamesWithReviewCount,
	}, nil
}

func (r *GamesRepo) CreateGames(games []ProductRepo) error {
	sql, _, err := r.db.Insert("games").Rows(games).
		OnConflict(goqu.DoNothing()).ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	start := time.Now()
	res, err := r.db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	took := time.Since(start)

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affectedRows != 0 {
		log.Printf("Inserted: %d/%d games from batch in: %v\n",
			affectedRows, len(games), took)
	}
	return nil
}

func (r *GamesRepo) CreateReviews(reviews []ReviewRepo) error {
	sql, _, err := r.db.Insert("reviews").Rows(reviews).
		OnConflict(goqu.DoNothing()).ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	start := time.Now()
	res, err := r.db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	took := time.Since(start)

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affectedRows != 0 {
		log.Printf("Inserted: %d/%d reviews from batch in: %v\n",
			affectedRows, len(reviews), took)
	}
	return nil
}

func CountGames(db *goqu.Database) (int, error) {
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

func (r *GamesRepo) CountReviewsForGame(id string) (int, error) {
	rows, err := r.countRevsForGame.Query(id)
	if err != nil {
		return -1, fmt.Errorf("failed to query revs for game: %s: %w",
			id, err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err = rows.Scan(&count); err != nil {
			return -1, fmt.Errorf("failed to scan: %w", err)
		}
	}

	return count, nil
}

func (r *GamesRepo) CountGames() (int, error) {
	rows, err := r.countGames.Query()
	if err != nil {
		return -1, fmt.Errorf("failed to query games count: %w", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err = rows.Scan(&count); err != nil {
			return -1, fmt.Errorf("failed to scan: %w", err)
		}
	}

	return count, nil
}

func (r *GamesRepo) GamesWithRevsiewsCount(offset, limit int) ([]UIGame, error) {
	rows, err := r.gamesWithRevsCount.Query(offset, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query with offset: %d and limit: %d: %w",
			offset, limit, err)
	}

	var games []UIGame
	for rows.Next() {
		var game UIGame
		err := rows.Scan(&game.ID, &game.Title, &game.ReviewsCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (r *GamesRepo) SaveNow() error {
	ts := time.Now().UTC().Format("2006-01-02 15:04:05.999")
	// Upsert if there's record already
	const query = `
		INSERT INTO last_run (onerow_id, ts) VALUES (true, $1)
		ON CONFLICT (onerow_id) DO UPDATE SET ts = $1
	`
	_, err := r.db.Exec(query, ts)
	return err
}

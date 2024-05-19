package goger

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

type GamesRepo struct {
	db                      *goqu.Database
	countGames              *sqlx.Stmt
	countRevsForGame        *sqlx.Stmt
	gamesWithRevsCount      *sqlx.Stmt
	reviewsByGame           *sqlx.Stmt
	trendingGamesNReviewsTs *sqlx.Stmt
}

func NewRepo() (*GamesRepo, error) {
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

	db, err := sqlx.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open pg connection: %w", err)
	}
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping pg: %w", err)
	}

	gq := goqu.New("postgres", db)
	countR, err := db.Preparex("SELECT COUNT(id) FROM reviews where product_id = $1")
	if err != nil {
		return nil, err
	}
	countG, err := db.Preparex("SELECT COUNT(id) FROM games")
	if err != nil {
		return nil, err
	}
	gamesWithReviewCount, err := db.Preparex(`
		SELECT id, title, reviews_count
		FROM games
		ORDER BY reviews_count DESC, id
		OFFSET $1
		LIMIT $2`)
	if err != nil {
		return nil, err
	}
	reviewsByGame, err := db.Preparex(`
		SELECT id, product_id, rating_value, title, description, language,
			reviewer_username, counters_games, counters_reviews, labels, downvotes,
			upvotes, review_date, creation_date, internal_update_date
		FROM reviews
		WHERE product_id = $1
		ORDER BY review_date DESC
		OFFSET $2
		LIMIT $3`)
	if err != nil {
		return nil, err
	}
	trendingGamesNReviewsTs, err := db.Preparex(`
	WITH reviews_last_hours AS (
		SELECT 
			product_id,
			review_date
		FROM reviews
		WHERE review_date >= NOW() - INTERVAL '1 HOUR' * $1
	),
	total_reviews AS (
		SELECT
			product_id,
			COUNT(*) AS total_reviews,
			ARRAY_AGG(review_date ORDER BY review_date DESC) AS review_dates
		FROM reviews_last_hours
		GROUP BY product_id
	)
	SELECT
		g.id,
		g.title,
		g.reviews_rating,
		tr.total_reviews,
		tr.review_dates
	FROM total_reviews tr
	JOIN games g ON g.id = tr.product_id
	ORDER BY tr.total_reviews DESC
	LIMIT $2;	
	`)
	if err != nil {
		return nil, err
	}

	return &GamesRepo{
		db:                      gq,
		countRevsForGame:        countR,
		countGames:              countG,
		gamesWithRevsCount:      gamesWithReviewCount,
		reviewsByGame:           reviewsByGame,
		trendingGamesNReviewsTs: trendingGamesNReviewsTs,
	}, nil
}

func (r *GamesRepo) CreateGames(games []ProductRepo) (int, error) {
	sql, _, err := r.db.Insert("games").Rows(games).
		OnConflict(goqu.DoNothing()).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	start := time.Now()
	res, err := r.db.Exec(sql)
	if err != nil {
		return 0, fmt.Errorf("failed to execute query: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get number of affected rows: %w", err)
	}
	took := time.Since(start)

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affectedRows != 0 {
		log.Printf("Inserted: %d/%d games from batch in: %v\n",
			affectedRows, len(games), took)
	}
	return int(affected), nil
}

func (r *GamesRepo) CreateReviews(reviews []ReviewRepo) (int, error) {
	// TODO: Config
	dbTimeout := 15 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin tx: %w", err)
	}

	sql, _, err := tx.Insert("reviews").Rows(reviews).
		OnConflict(goqu.DoNothing()).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("failed to build insert query: %w", err)
	}
	res, err := tx.ExecContext(ctx, sql)
	if err != nil {
		return 0, fmt.Errorf("failed to execute insert query: %w", err)
	}

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected by insert: %w", err)
	}
	if affectedRows == 0 {
		if err = tx.Commit(); err != nil {
			err = fmt.Errorf("failed to commit tx: %w", err)
		}
		return 0, err
	}
	numNewReviews := int(affectedRows)

	sql, _, err = tx.Update("games").
		Set(goqu.Record{"reviews_count": goqu.L("reviews_count + ?", affectedRows)}).
		Where(goqu.Ex{"id": reviews[0].ProductID}).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("failed to build reviews count update query: %w", err)
	}
	_, err = tx.ExecContext(ctx, sql)
	if err != nil {
		return 0, fmt.Errorf("failed to exec reviews count update: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit final tx: %w", err)
	}

	return numNewReviews, nil
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

func (r *GamesRepo) GameReviewsCount(id string) (int, error) {
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

func (r *GamesRepo) GamesWithReviewsCount(offset, limit int) ([]UIGame, error) {
	rows, err := r.gamesWithRevsCount.Query(offset, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query with offset: %d and limit: %d: %w",
			offset, limit, err)
	}
	defer rows.Close()

	games := make([]UIGame, 0, limit)
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

func (r *GamesRepo) MostReviewedGamesWithRevTs(hours, limit int) ([]TrendingGame, error) {
	rows, err := r.trendingGamesNReviewsTs.Query(hours, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query with hours %d and limit: %d: %w",
			hours, limit, err)
	}
	defer rows.Close()

	games := make([]TrendingGame, 0, limit)
	for rows.Next() {
		var game TrendingGame
		err := rows.Scan(
			&game.ID,
			&game.Title,
			&game.ReviewsRating,
			&game.TotalReviews,
			&game.ReviewDates,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (r *GamesRepo) ReviewsForGame(productID, offset, limit int) (RepoReviews, error) {
	rows, err := r.reviewsByGame.Queryx(productID, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query reviews for product ID %d "+
			"with offset: %d and limit: %d: %w",
			productID, offset, limit, err)
	}
	defer rows.Close()

	reviews := make([]ReviewRepo, 0, limit)
	for rows.Next() {
		var review ReviewRepo
		if err := rows.StructScan(&review); err != nil {
			return nil, fmt.Errorf("failed to scan review: %w", err)
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
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

func (r *GamesRepo) CreateRun(run RunRepo) error {
	sql, _, err := r.db.Insert("run").Rows(run).ToSQL()
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	res, err := r.db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to insert err: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("want 1 affected row got: %d", affected)
	}

	return nil
}

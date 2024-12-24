package goger

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

const dateLayout = "2006-01-02 15:04:05-07"

type GamesRepo struct {
	logger                     *slog.Logger
	db                         *goqu.Database
	countGames                 *sqlx.Stmt
	countRevsForGame           *sqlx.Stmt
	gamesWithRevsCount         *sqlx.Stmt
	reviewsByGame              *sqlx.Stmt
	trendingGamesNReviewsTs    *sqlx.Stmt
	totalGames                 *sqlx.Stmt
	avgRatings                 *sqlx.Stmt
	mostPopularGames           *sqlx.Stmt
	mostPositiveGames          *sqlx.Stmt
	mostNegativeGames          *sqlx.Stmt
	avgReviewsPerGame          *sqlx.Stmt
	topLanguages               *sqlx.Stmt
	topRatingVals              *sqlx.Stmt
	avgReviewsPerUser          *sqlx.Stmt
	mostReviewsPerUser         *sqlx.Stmt
	gamesWithMostReviewsIn1Day *sqlx.Stmt
	topYearToGamesReleased     *sqlx.Stmt
	dayWithMostReviews         *sqlx.Stmt
	gamesPerDeveloper          *sqlx.Stmt
}

func NewRepo(l *slog.Logger) (*GamesRepo, error) {
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
	countR, err := db.Preparex("SELECT COUNT(id) FROM reviews WHERE product_id = $1")
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

	totalGames, err := db.Preparex(`
    SELECT COUNT(*) AS total_games FROM games
`)
	if err != nil {
		return nil, err
	}

	avgRating, err := db.Preparex(`
    SELECT AVG(rating_value) AS average_rating FROM reviews
`)
	if err != nil {
		return nil, err
	}

	mostPopularGames, err := db.Preparex(`
    SELECT id, title, reviews_count, reviews_rating, TO_CHAR(release_date, 'DD-MM-YYYY') AS release_date, developers
    FROM games
    ORDER BY reviews_count DESC
    LIMIT $1
`)
	if err != nil {
		return nil, err
	}

	mostPositiveGames, err := db.Preparex(`
	SELECT id, title, reviews_count, reviews_rating, TO_CHAR(release_date, 'DD-MM-YYYY') AS release_date, developers
	FROM games
	WHERE reviews_count > 0
	ORDER BY reviews_rating DESC, reviews_count DESC
	LIMIT $1;
`)
	if err != nil {
		return nil, err
	}

	mostNegativeGames, err := db.Preparex(`
	SELECT id, title, reviews_count, reviews_rating, TO_CHAR(release_date, 'DD-MM-YYYY') AS release_date, developers
	FROM games
	WHERE reviews_count > 0 AND reviews_rating > 0
	ORDER BY reviews_rating ASC, reviews_count DESC
	LIMIT $1;
`)
	if err != nil {
		return nil, err
	}

	avgReviewsPerGame, err := db.Preparex(`
    SELECT ROUND(AVG(reviews_count), 2) AS average_reviews_per_game FROM games
`)
	if err != nil {
		return nil, err
	}

	gamesWithMostReviewsIn1Day, err := db.Preparex(`
WITH daily_reviews AS (
    SELECT product_id, DATE(review_date) AS review_date, COUNT(*) AS review_count, AVG(rating_value) AS average_rating
    FROM reviews
    GROUP BY product_id, DATE(review_date)
),
max_daily_reviews AS (
    SELECT product_id, review_date, review_count, average_rating,
           ROW_NUMBER() OVER (PARTITION BY product_id ORDER BY review_count DESC) AS rn
    FROM daily_reviews
)
SELECT g.title, 
TO_CHAR(mdr.review_date, 'DD-MM-YYYY') AS review_date, TO_CHAR(g.release_date, 'DD-MM-YYYY') AS release_date,
mdr.review_count AS total_reviews, ROUND(mdr.average_rating, 2) AS rating
FROM max_daily_reviews mdr
JOIN games g ON g.id = mdr.product_id
WHERE mdr.rn = 1
ORDER BY total_reviews DESC
LIMIT $1;
	`)
	if err != nil {
		return nil, err
	}

	topLanguages, err := db.Preparex(`
SELECT split_part(language, '-', 2) AS language, COUNT(*) AS review_count
FROM reviews
GROUP BY language
ORDER BY review_count DESC
LIMIT $1;
		`)
	if err != nil {
		return nil, err
	}

	topRatingVal, err := db.Preparex(`
SELECT rating_value, COUNT(*) AS count
FROM reviews
GROUP BY rating_value
ORDER BY count DESC
LIMIT $1;
`)
	if err != nil {
		return nil, err
	}

	avgReviewsPerUser, err := db.Preparex(`
SELECT ROUND(AVG(review_count), 2) AS average_reviews_per_user
FROM (
    SELECT reviewer_id, COUNT(*) AS review_count
    FROM reviews
    GROUP BY reviewer_id
) AS user_reviews;
		`)
	if err != nil {
		return nil, err
	}

	mostReviewsPerUser, err := db.Preparex(`
SELECT COUNT(id) AS review_count
  FROM reviews
  GROUP BY reviewer_id
  ORDER BY review_count DESC
  LIMIT 1;
		`)
	if err != nil {
		return nil, err
	}

	topYearToGamesReleased, err := db.Preparex(`
SELECT EXTRACT(YEAR FROM release_date) AS year, COUNT(*) AS number_of_games_released
FROM games
GROUP BY year
ORDER BY year DESC
LIMIT $1;
		`)
	if err != nil {
		return nil, err
	}

	dayWithMostReviews, err := db.Preparex(`
SELECT 
    DATE(review_date) AS review_day, 
    COUNT(*) AS review_count
FROM 
    reviews
GROUP BY 
    review_day
ORDER BY 
    review_count DESC
LIMIT 1;
		`)
	if err != nil {
		return nil, err
	}

	gamesPerDeveloper, err := db.Preparex(`
SELECT 
    UNNEST(developers) AS developer,
    COUNT(*) AS games_count
FROM 
    games
GROUP BY 
    developer
ORDER BY 
    games_count DESC
LIMIT $1;
		`)
	if err != nil {
		return nil, err
	}

	return &GamesRepo{
		logger:                     l,
		db:                         gq,
		countRevsForGame:           countR,
		countGames:                 countG,
		gamesWithRevsCount:         gamesWithReviewCount,
		reviewsByGame:              reviewsByGame,
		trendingGamesNReviewsTs:    trendingGamesNReviewsTs,
		totalGames:                 totalGames,
		avgRatings:                 avgRating,
		mostPopularGames:           mostPopularGames,
		mostPositiveGames:          mostPositiveGames,
		mostNegativeGames:          mostNegativeGames,
		avgReviewsPerGame:          avgReviewsPerGame,
		gamesWithMostReviewsIn1Day: gamesWithMostReviewsIn1Day,
		topLanguages:               topLanguages,
		topRatingVals:              topRatingVal,
		avgReviewsPerUser:          avgReviewsPerUser,
		topYearToGamesReleased:     topYearToGamesReleased,
		mostReviewsPerUser:         mostReviewsPerUser,
		dayWithMostReviews:         dayWithMostReviews,
		gamesPerDeveloper:          gamesPerDeveloper,
	}, nil
}

// CreateGames inserts multiple games into the "games" table.
//
// It takes a slice of ProductRepo as a parameter, which represents the games to be inserted.
// The function returns the number of rows affected by the insertion and an error if any.
func (r *GamesRepo) CreateGames(games []ProductRepo) (int, error) {
	sql, _, err := r.db.Insert("games").Rows(games).
		OnConflict(goqu.DoUpdate("id",
			goqu.Record{
				"slug":                             goqu.L("EXCLUDED.slug"),
				"features":                         goqu.L("EXCLUDED.features"),
				"screenshots":                      goqu.L("EXCLUDED.screenshots"),
				"user_preferred_language_code":     goqu.L("EXCLUDED.user_preferred_language_code"),
				"user_preferred_language_in_audio": goqu.L("EXCLUDED.user_preferred_language_in_audio"),
				"user_preferred_language_in_text":  goqu.L("EXCLUDED.user_preferred_language_in_text"),
				"release_date":                     goqu.L("EXCLUDED.release_date"),
				"store_release_date":               goqu.L("EXCLUDED.store_release_date"),
				"product_type":                     goqu.L("EXCLUDED.product_type"),
				"title":                            goqu.L("EXCLUDED.title"),
				"cover_horizontal":                 goqu.L("EXCLUDED.cover_horizontal"),
				"cover_vertical":                   goqu.L("EXCLUDED.cover_vertical"),
				"developers":                       goqu.L("EXCLUDED.developers"),
				"publishers":                       goqu.L("EXCLUDED.publishers"),
				"operating_systems":                goqu.L("EXCLUDED.operating_systems"),
				"price_final":                      goqu.L("EXCLUDED.price_final"),
				"price_base":                       goqu.L("EXCLUDED.price_base"),
				"price_currency":                   goqu.L("EXCLUDED.price_currency"),
				"price_discount":                   goqu.L("EXCLUDED.price_discount"),
				"product_state":                    goqu.L("EXCLUDED.product_state"),
				"genres":                           goqu.L("EXCLUDED.genres"),
				"tags":                             goqu.L("EXCLUDED.tags"),
				"reviews_rating":                   goqu.L("EXCLUDED.reviews_rating"),
				"updated_at":                       goqu.L("EXCLUDED.updated_at"),
			})).ToSQL()
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

	if affected != 0 {
		r.logger.Debug("Inserted games batch",
			"affected", affected, "total", len(games), "took", took)
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
	r.logger.Info("Added reviews", "count", affectedRows)
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

func (r *GamesRepo) MostReviewedGamesWithRevTs(hoursAgo, limit int) (TrendingGames, error) {
	rows, err := r.trendingGamesNReviewsTs.Query(hoursAgo, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query with hours %d and limit: %d: %w",
			hoursAgo, limit, err)
	}
	defer rows.Close()

	games := make([]TrendingGame, 0, limit)
	err = sqlx.StructScan(rows, &games)
	if err != nil {
		return nil, fmt.Errorf("failed to scan game: %w", err)
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

func (r *GamesRepo) AverageReviewsPerGame() (float64, error) {
	rows, err := r.avgReviewsPerGame.Query()
	if err != nil {
		return -1, fmt.Errorf("failed to query avg reviews per game: %w", err)
	}
	defer rows.Close()

	var avg float64
	for rows.Next() {
		if err = rows.Scan(&avg); err != nil {
			return -1, fmt.Errorf("failed to scan: %w", err)
		}
	}

	return avg, nil
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

func (r *GamesRepo) MostPositiveGames(limit int) (ProductsRepo, error) {
	rows, err := r.mostPositiveGames.Query(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to exec most positive games query: %w", err)
	}
	defer rows.Close()

	var games []ProductRepo
	err = sqlx.StructScan(rows, &games)
	if err != nil {
		return nil, fmt.Errorf("failed to scan most positive games: %w", err)
	}

	return games, nil
}

func (r *GamesRepo) MostNegativeGames(limit int) (ProductsRepo, error) {
	rows, err := r.mostNegativeGames.Query(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to exec most negative games query: %w", err)
	}
	defer rows.Close()

	var games []ProductRepo
	err = sqlx.StructScan(rows, &games)
	if err != nil {
		return nil, fmt.Errorf("failed to scan most negative games: %w", err)
	}

	return games, nil
}

func (r *GamesRepo) MostPopularGames(limit int) (ProductsRepo, error) {
	rows, err := r.mostPopularGames.Query(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to exec most popular games query: %w", err)
	}
	defer rows.Close()

	var games []ProductRepo
	err = sqlx.StructScan(rows, &games)
	if err != nil {
		return nil, fmt.Errorf("failed to scan most popular games: %w", err)
	}

	return games, nil
}

func (r *GamesRepo) GamesWithMostReviewsIn1Day(limit int) ([]GameWithMostReviewsIn1Day, error) {
	rows, err := r.gamesWithMostReviewsIn1Day.Query(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to exec games with most reviews in 1 day: %w", err)
	}
	defer rows.Close()

	var games []GameWithMostReviewsIn1Day
	err = sqlx.StructScan(rows, &games)
	if err != nil {
		return nil, fmt.Errorf("failed to scan games with most reviews in 1 day: %w", err)
	}

	return games, nil
}

func (r *GamesRepo) TopLanguages(limit int) ([]LanguageCount, error) {
	rows, err := r.topLanguages.Query(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top languages: %w", err)
	}
	defer rows.Close()

	var langs []LanguageCount
	err = sqlx.StructScan(rows, &langs)
	if err != nil {
		return nil, fmt.Errorf("failed to scan top languages: %w", err)
	}
	return langs, nil
}

func (r *GamesRepo) TopRatingVals(limit int) ([]RatingVal, error) {
	rows, err := r.topRatingVals.Query(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query rating vals: %w", err)
	}
	defer rows.Close()

	var vals []RatingVal
	err = sqlx.StructScan(rows, &vals)
	if err != nil {
		return nil, fmt.Errorf("failed to scan top languages: %w", err)
	}

	return vals, nil
}

func (r *GamesRepo) AvgReviewsPerUser() (float64, error) {
	var avg float64
	return avg, r.avgReviewsPerUser.Get(&avg)
}

func (r *GamesRepo) AvgReviewsPerGame() (float64, error) {
	var avg float64
	return avg, r.avgReviewsPerGame.Get(&avg)
}

func (r *GamesRepo) MostReviewsPerUser() (int, error) {
	var most int
	return most, r.mostReviewsPerUser.Get(&most)
}

func (r *GamesRepo) DayWithMostReviews() (time.Time, int, error) {
	aux := struct {
		Day   time.Time `db:"review_day"`
		Count int       `db:"review_count"`
	}{}
	err := r.dayWithMostReviews.QueryRowx().StructScan(&aux)
	return aux.Day, aux.Count, err
}

func (r *GamesRepo) GamesPerDeveloper(limit int) ([][2]string, error) {
	aux := []struct {
		GamesCount int64  `db:"games_count"`
		Developer  string `db:"developer"`
	}{}
	if err := r.gamesPerDeveloper.Select(&aux, limit); err != nil {
		return nil, fmt.Errorf("failed to select games count per dev: %w", err)
	}

	gamesCountToDev := make([][2]string, 0, len(aux))
	for _, v := range aux {
		gamesCountToDev = append(gamesCountToDev,
			[2]string{v.Developer, strconv.FormatInt(v.GamesCount, 10)},
		)
	}

	return gamesCountToDev, nil
}

func (r *GamesRepo) TopYearToGamesReleased(limit int) ([]GamesReleasedByYear, error) {
	rows, err := r.topYearToGamesReleased.Query(limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query rating vals: %w", err)
	}
	defer rows.Close()

	var vals []GamesReleasedByYear
	err = sqlx.StructScan(rows, &vals)
	if err != nil {
		return nil, fmt.Errorf("failed to scan top languages: %w", err)
	}

	return vals, nil
}

func (r *GamesRepo) Game(title string) (ProductRepo, error) {
	var game ProductRepo
	_, err := r.db.From("games").Where(goqu.Ex{"title": title}).ScanStruct(&game)
	return game, err
}

func (r *GamesRepo) MostCommonLanguages(title string, limit uint) ([]LanguageCount, error) {
	ds := r.db.From("games").
		Join(goqu.T("reviews"), goqu.On(goqu.Ex{"games.id": goqu.I("reviews.product_id")})).
		Select(
			goqu.L("split_part(language, '-', 2)").As("language"),
			goqu.COUNT("reviews.language").As("review_count"),
		).
		Where(goqu.Ex{"games.title": title}).
		GroupBy(goqu.I("reviews.language")).
		Order(goqu.COUNT("reviews.language").Desc()).
		Limit(limit)

	var counts []LanguageCount
	err := ds.ScanStructs(&counts)
	if err != nil {
		return nil, fmt.Errorf("failed to scan array of LanguageCount: %w", err)
	}

	return counts, nil
}

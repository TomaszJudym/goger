package goger

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

const dateLayout = "2006-01-02 15:04:05-07"

type GamesRepo struct {
	logger *slog.Logger
	db     *goqu.Database
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
	return &GamesRepo{
		logger: l,
		db:     goqu.New("postgres", db),
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

	// Grab a title for log
	title := ""
	_, err = r.db.From("games").
		Select("title").
		Where(goqu.Ex{"id": reviews[0].ProductID}).
		Limit(1).
		ScanVal(&title)
	if err != nil {
		r.logger.Error("Failed to get game", "id", reviews[0].ProductID, "err", err)
	}
	r.logger.Debug("Inserted reviews", "count", affectedRows, "title", title)
	return numNewReviews, nil
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

func (r *GamesRepo) GameReviewsCount(id string) (int, error) {
	const query = `
		SELECT reviews_count
		FROM games
		WHERE id = $1;
	`
	var reviewsCount int
	err := r.db.QueryRow(query, id).Scan(&reviewsCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("game with ID %s not found", id)
		}
		return 0, fmt.Errorf("failed to execute query: %w", err)
	}
	return reviewsCount, nil
}

func (r *GamesRepo) Game(title string) (ProductRepo, error) {
	var game ProductRepo
	_, err := r.db.From("games").Where(goqu.Ex{"title": title}).ScanStruct(&game)
	return game, err
}

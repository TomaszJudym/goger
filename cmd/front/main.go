package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// timeRFC3339 is a type alias for time.Time with custom JSON unmarshaling.
type timeRFC3339 time.Time

// UnmarshalJSON implements custom JSON unmarshaling for CustomTime.
func (ct *timeRFC3339) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	// Add "Z" if no timezone is present, assuming UTC.
	if len(s) == 19 || len(s) == 26 {
		s += "Z"
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}

	*ct = timeRFC3339(t)
	return nil
}

func (t timeRFC3339) String() string {
	return time.Time(t).Format(time.RFC3339)
}

type timeDate time.Time

// UnmarshalJSON implements custom JSON unmarshaling for CustomTime.
func (ct *timeDate) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}

	*ct = timeDate(t)
	return nil
}

func (t timeDate) String() string {
	return time.Time(t).Format("2006-01-02")
}

// GameStatistics represents the overall statistics for games.
type GameStatistics struct {
	TotalGames           int                  `db:"total_games"`
	TotalReviews         int                  `db:"total_reviews"`
	AvgGameRating        float64              `db:"avg_game_rating"`
	GamesWithReviews     int                  `db:"games_with_reviews"`
	PriceStatistics      PriceStatistics      `db:"price_statistics"`
	TopGenres            TopGenres            `db:"top_genres"`
	OsDistribution       OsDistribution       `db:"os_distribution"`
	TopPublishers        TopPublishers        `db:"top_publishers"`
	TopDevelopers        TopDevelopers        `db:"top_developers"`
	ReleaseTrends        ReleaseTrends        `db:"release_trends"`
	MostReviewedGames    MostReviewedGames    `db:"most_reviewed_games"`
	HighestRatedGames    HighestRatedGames    `db:"highest_rated_games"`
	RecentlyUpdatedGames RecentlyUpdatedGames `db:"recently_updated_games"`
	ViewRefreshTime      timeRFC3339          `db:"view_refresh_time"`
}

// PriceStatistics represents price statistics by currency.
type PriceStatistics []struct {
	Currency    string  `json:"currency"`
	AvgPrice    float64 `json:"avg_price"`
	MaxPrice    float64 `json:"max_price"`
	MinPrice    float64 `json:"min_price"`
	TotalGames  int     `json:"total_games"`
	AvgDiscount float64 `json:"avg_discount"`
	GamesOnSale int     `json:"games_on_sale"`
}

// scanJSON converts a SQL value to a byte slice.
func scanJSON(src any, v any) error {
	if src == nil {
		return nil
	}
	byteValue, err := toByteSlice(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(byteValue, v)
}

// toByteSlice converts a SQL value to a byte slice.
func toByteSlice(src any) ([]byte, error) {
	switch src := src.(type) {
	case []byte:
		return src, nil
	case string:
		return []byte(src), nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", src)
	}
}

// Scan implements the sql.Scanner interface for PriceStatistics.
func (p *PriceStatistics) Scan(src any) error {
	return scanJSON(src, p)
}

// TopGenres represents the top genres.
type TopGenres []struct {
	Genre     string  `json:"genre"`
	GameCount int     `json:"game_count"`
	AvgRating float64 `json:"avg_rating"`
	AvgPrice  float64 `json:"avg_price"`
}

// Scan implements the sql.Scanner interface for TopGenres.
func (t *TopGenres) Scan(src any) error {
	return scanJSON(src, t)
}

// OsDistribution represents the operating system distribution.
type OsDistribution []struct {
	Os        string `json:"os"`
	GameCount int    `json:"game_count"`
}

// Scan implements the sql.Scanner interface for OsDistribution.
func (o *OsDistribution) Scan(src any) error {
	return scanJSON(src, o)
}

// TopPublishers represents the top publishers.
type TopPublishers []struct {
	Publisher      string  `json:"publisher"`
	PublishedGames int     `json:"published_games"`
	AvgRating      float64 `json:"avg_rating"`
}

// Scan implements the sql.Scanner interface for TopPublishers.
func (t *TopPublishers) Scan(src any) error {
	return scanJSON(src, t)
}

// TopDevelopers represents the top developers.
type TopDevelopers []struct {
	Developer      string  `json:"developer"`
	DevelopedGames int     `json:"developed_games"`
	AvgRating      float64 `json:"avg_rating"`
}

// Scan implements the sql.Scanner interface for TopDevelopers.
func (t *TopDevelopers) Scan(src any) error {
	return scanJSON(src, t)
}

// ReleaseTrends represents the release trends.
type ReleaseTrends []struct {
	Month         timeRFC3339 `json:"month"`
	GamesReleased int         `json:"games_released"`
	AvgRating     float64     `json:"avg_rating"`
}

// Scan implements the sql.Scanner interface for ReleaseTrends.
func (r *ReleaseTrends) Scan(src any) error {
	return scanJSON(src, r)
}

// MostReviewedGames represents the most reviewed games.
type MostReviewedGames []struct {
	Id           int      `json:"id"`
	Title        string   `json:"title"`
	ReviewsCount int      `json:"reviews_count"`
	Rating       int      `json:"rating"`
	ReleaseDate  timeDate `json:"release_date"`
}

// Scan implements the sql.Scanner interface for MostReviewedGames.
func (m *MostReviewedGames) Scan(src any) error {
	return scanJSON(src, m)
}

// HighestRatedGames represents the highest rated games.
type HighestRatedGames []struct {
	Id           int      `json:"id"`
	Title        string   `json:"title"`
	ReviewsCount int      `json:"reviews_count"`
	Rating       int      `json:"rating"`
	ReleaseDate  timeDate `json:"release_date"`
}

// Scan implements the sql.Scanner interface for HighestRatedGames.
func (h *HighestRatedGames) Scan(src any) error {
	return scanJSON(src, h)
}

// RecentlyUpdatedGames represents the recently updated games.
type RecentlyUpdatedGames []struct {
	Id        int         `json:"id"`
	Title     string      `json:"title"`
	UpdatedAt timeRFC3339 `json:"updated_at"`
}

// Scan implements the sql.Scanner interface for RecentlyUpdatedGames.
func (r *RecentlyUpdatedGames) Scan(src any) error {
	return scanJSON(src, r)
}

func main() {
	http.HandleFunc("/", dashboardHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, fmt.Sprintf("failed to open pg connection %v", err), http.StatusInternalServerError)
		return
	}
	if err = db.Ping(); err != nil {
		http.Error(w, fmt.Sprintf("failed to ping pg: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = db.Exec("REFRESH MATERIALIZED VIEW game_statistics")
	if err != nil {
		log.Fatalf("Error refreshing materialized view: %v", err)
	}

	var data GameStatistics
	if err = db.Get(&data, `select * from game_statistics`); err != nil {
		http.Error(w, fmt.Sprintf("Error calling view: %v", err), http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse: %v", err), http.StatusInternalServerError)
		return
	}

	if err = tmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Execute: %v", err), http.StatusInternalServerError)
		return
	}

}

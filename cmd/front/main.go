package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	svg "github.com/ajstarks/svgo"
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
	SvgData              template.HTML
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

	var byteValue []byte
	switch src := src.(type) {
	case []byte:
		byteValue = src
	case string:
		byteValue = []byte(src)
	default:
		return fmt.Errorf("unsupported type: %T", src)
	}

	return json.Unmarshal(byteValue, v)
}

type DataPoint struct {
	Date  time.Time `db:"review_day"`
	Value int       `db:"total_reviews"`
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

	svgBuff, err := newReviewsSVG(db)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to build reviews SVG: %v", err), http.StatusInternalServerError)
		return
	}
	data.SvgData = template.HTML(svgBuff.String())

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

func newReviewsSVG(db *sqlx.DB) (*bytes.Buffer, error) {
	data, err := reviewData(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get reviews data: %w", err)
	}
	return newSVGChart(data)
}

func reviewData(db *sqlx.DB) ([]DataPoint, error) {
	query := `
                SELECT
                        DATE_TRUNC('day', review_date) AS review_day,
                        COUNT(*) AS total_reviews
                FROM
                        reviews
                WHERE
                        review_date IS NOT NULL
                GROUP BY
                        review_day
                ORDER BY
                        review_day;
        `

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []DataPoint
	for rows.Next() {
		var dp DataPoint
		if err := rows.Scan(&dp.Date, &dp.Value); err != nil {
			return nil, err
		}
		data = append(data, dp)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return data, nil
}

func newSVGChart(data []DataPoint) (*bytes.Buffer, error) {
	const (
		chartWidth  = 600
		chartHeight = 400
	)
	// TODO: Make graph fit page width.
	// Don't display all dates on X axis
	if len(data) > 100 {
		data = data[:100]
	}

	var buf bytes.Buffer
	canvas := svg.New(&buf)
	canvas.Start(chartWidth, chartHeight)

	// Background color
	backgroundColor := "rgb(20, 0, 40)"
	canvas.Rect(0, 0, chartWidth, chartHeight, fmt.Sprintf("fill:%s", backgroundColor))

	// Axis color
	axisColor := "rgb(221, 160, 221)"

	// Axis positions
	axisMargin := 40
	axisY := chartHeight - axisMargin
	axisX := axisMargin

	// Draw x-axis
	canvas.Line(axisX, axisY, chartWidth-axisMargin, axisY, fmt.Sprintf("stroke:%s; stroke-width:2", axisColor))

	// Draw y-axis
	canvas.Line(axisX, axisMargin, axisX, chartHeight-axisMargin, fmt.Sprintf("stroke:%s; stroke-width:2", axisColor))

	// Draw Data Points as vertical lines
	if len(data) > 0 {
		maxValue := 0
		for _, dp := range data {
			if dp.Value > maxValue {
				maxValue = dp.Value
			}
		}

		lineColor := "rgb(153, 50, 204)" // Light purple lines
		lineWidth := 2

		// Calculate scaling factors
		xScale := float64(chartWidth-2*axisMargin) / float64(len(data)-1)
		yScale := float64(chartHeight-2*axisMargin) / float64(maxValue)

		for i, dp := range data {
			x := int(float64(i)*xScale) + axisMargin
			y := chartHeight - axisMargin - int(float64(dp.Value)*yScale)

			canvas.Line(x, chartHeight-axisMargin, x, y, fmt.Sprintf("stroke:%s; stroke-width:%d", lineColor, lineWidth))                            // Draw vertical lines
			canvas.Text(x, chartHeight-axisMargin+15, dp.Date.Format("01-02"), fmt.Sprintf("fill:%s; text-anchor:middle; font-size:8px", axisColor)) // Add date labels
		}
	}

	canvas.End()
	return &buf, nil
}

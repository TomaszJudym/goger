package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/tomaszjudym/goger"
)

type Repo interface {
	ReviewsForGame(title string, offset, limit int) (goger.RepoReviews, error)
	CountGames() (int, error)
	GameReviewsCount(gameID string) (int, error)
	MostReviewedGamesWithRevTs(hours, limit int) (goger.TrendingGames, error)
	AverageReviewsPerGame() (float64, error)
	MostPopularGames(limit int) (goger.ProductsRepo, error)
	MostPositiveGames(limit int) (goger.ProductsRepo, error)
	MostNegativeGames(limit int) (goger.ProductsRepo, error)
	TopLanguages(limit int) ([]goger.LanguageCount, error)
	TopRatingVals(limit int) ([]goger.RatingVal, error)
	GamesWithMostReviewsIn1Day(limit int) ([]goger.GameWithMostReviewsIn1Day, error)
	AvgReviewsPerUser() (float64, error)
	AvgReviewsPerGame() (float64, error)
	MostReviewsPerUser() (int, error)
	TopYearToGamesReleased(limit int) ([]goger.GamesReleasedByYear, error)
	DayWithMostReviews() (time.Time, int, error)
	GamesPerDeveloper(limit int) ([][2]string, error)
	Game(title string) (goger.ProductRepo, error)
	MostCommonLanguages(title string, limit uint) ([]goger.LanguageCount, error)
	ReviewsStats(title string) (goger.ReviewStats, error)
}

var (
	db        Repo
	rdb       *redis.Client
	indexTmpl *template.Template
)

func init() {
	var err error
	db, err = goger.NewRepo(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "",
		DB:       0, // Use default DB
	})

	indexTmpl, err = template.ParseFiles("templates/index.html")
	if err != nil {
		log.Fatalf("Failed to parse index template: %v", err)
	}
}

func main() {
	e := echo.New()
	e.GET("/", handlerIndex)
	e.GET("/games/:title", handlerGame)
	e.GET("/reviews/:title", handlerReviews)
	e.File("favicon.png", "static/favicon.png")
	e.Static("/static", "static")
	fmt.Printf("Server running on: %s", ":8080")
	log.Fatal(e.Start(":8080"))
}

type IndexData struct {
	Trending            []goger.UIGame
	TrendingChart       []goger.UIGame
	Top                 []goger.UIGame
	TopNegative         []goger.UIGame
	TopPositive         []goger.UIGame
	TopGamesIn1Day      []goger.GameWithMostReviewsIn1Day
	TopLanguages        []goger.LanguageCount
	Ratings             []goger.RatingVal
	YearToGamesReleased []goger.GamesReleasedByYear
	AvgReviewsPerUser   float64
	AvgReviewsPerGame   float64
	MostReviewsPerUser  int
	MostReviewsDay      string
	MostReviewsIn1Day   int
	GamesPerDeveloper   [][2]string
}

type DataGame struct {
	Title       string
	Items       [][2]string // key -> vals
	Languages   []goger.LanguageCount
	Screenshots []string
}

func handlerIndex(c echo.Context) error {
	start := time.Now()

	// Get page parameter from the query string
	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page < 1 {
		page = 1
	}

	trendingGames, err := db.MostReviewedGamesWithRevTs(14*24, 5)
	if err != nil {
		log.Printf("Failed to get most reviewed games with rev ts: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	popularGames, err := db.MostPopularGames(5)
	if err != nil {
		log.Printf("Failed to get trending games: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	mostRevsIn1DayGames, err := db.GamesWithMostReviewsIn1Day(5)
	if err != nil {
		log.Printf("Failed to games with most reviews in 1 day: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	popularGames, err = db.MostPositiveGames(5)
	if err != nil {
		log.Printf("Failed to get most positive games: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	negativeGames, err := db.MostNegativeGames(5)
	if err != nil {
		log.Printf("Failed to get most negative games: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	topLanguages, err := db.TopLanguages(5)
	if err != nil {
		log.Printf("Failed to get top languages: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	topRatingVals, err := db.TopRatingVals(5)
	if err != nil {
		log.Printf("Failed to get top rating val: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	yearsToGames, err := db.TopYearToGamesReleased(100) // all
	if err != nil {
		log.Printf("Failed to get years to games released: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	avgRevsPerUser, err := db.AvgReviewsPerUser()
	if err != nil {
		log.Printf("Failed to get avg reviews per user: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	avgRevsPerGame, err := db.AvgReviewsPerGame()
	if err != nil {
		log.Printf("Failed to get avg reviews per user: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	mostReviewsPerUser, err := db.MostReviewsPerUser()
	if err != nil {
		log.Printf("Failed to get most reviews per user: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	mostReviewsDay, mostReviewsIn1Day, err := db.DayWithMostReviews()
	if err != nil {
		log.Printf("Failed to get day with most reviews: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	gamesPerDev, err := db.GamesPerDeveloper(10)
	if err != nil {
		log.Printf("Failed to get days per developer: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	data := IndexData{
		Trending:            trendingGames.ToUI(),
		Top:                 popularGames.ToUI(),
		TopGamesIn1Day:      mostRevsIn1DayGames,
		TopPositive:         popularGames.ToUI(),
		TopNegative:         negativeGames.ToUI(),
		TopLanguages:        topLanguages,
		Ratings:             topRatingVals,
		YearToGamesReleased: yearsToGames,
		AvgReviewsPerUser:   avgRevsPerUser,
		AvgReviewsPerGame:   avgRevsPerGame,
		MostReviewsPerUser:  mostReviewsPerUser,
		MostReviewsDay:      mostReviewsDay.Format("2006-01-02"),
		MostReviewsIn1Day:   mostReviewsIn1Day,
		GamesPerDeveloper:   gamesPerDev,
	}

	// Use Echo's Render function to return the template
	var buff bytes.Buffer
	if err = indexTmpl.Execute(&buff, data); err != nil {
		log.Printf("Failed to execute template: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	// Send the response
	if _, err = c.Response().Write(buff.Bytes()); err != nil {
		log.Printf("Failed to write response: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	log.Printf("Served games in %v, for IP: %s, URI: %s",
		time.Since(start), c.Request().RemoteAddr, c.Request().RequestURI)
	return nil
}

func handlerGame(c echo.Context) error {
	title := c.Param("title")
	if title == "" {
		return c.JSON(http.StatusBadRequest, "title param missing")
	}

	game, err := db.Game(title)
	if err != nil {
		log.Printf("failed to get game %s: %v", title, err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	languages, err := db.MostCommonLanguages(title, 5)
	if err != nil {
		log.Printf("failed to get most common languages: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	data := DataGame{
		Title: game.Title,
		Items: [][2]string{
			{"Reviews count", strconv.Itoa(game.ReviewsCount)},
			{"Reviews rating", strconv.Itoa(game.ReviewsRating)},
			{"Developers", strings.Join(game.Developers, "\n")},
			{"Publishers", strings.Join(game.Publishers, "\n")},
			{"Release date", game.ReleaseDate[:10]},
		},
		Languages:   languages,
		Screenshots: game.Screenshots,
	}

	tmpl, err := template.ParseFiles("templates/game.html")
	if err != nil {
		log.Printf("failed to parse game template: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	err = tmpl.Execute(c.Response().Writer, data)
	if err != nil {
		log.Printf("failed to execute game template: %v", err)
		return c.JSON(http.StatusInternalServerError, "Internal Server Error")
	}

	return nil
}

func handlerReviews(c echo.Context) error {
	title := c.Param("title")
	if title == "" {
		return c.JSON(http.StatusBadRequest, "title param missing")
	}

	stats, err := db.ReviewsStats(title)
	if err != nil {
		return internal(c, "failed to get review stats: %w", err)
	}

	marshalled, err := json.MarshalIndent(stats, " ", "\t")
	if err != nil {
		return internal(c, "xD1")
	}
	_, err = c.Response().Write(marshalled)
	if err != nil {
		return internal(c, "xD2")
	}
	return nil
}

func internal(c echo.Context, msg string, args ...any) error {
	log.Printf(msg, args...)
	return c.JSON(http.StatusBadRequest, "internal error")
}

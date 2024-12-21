package main

import (
	"bytes"
	"context"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/lib/pq"
	"github.com/tomaszjudym/goger"
)

// TODO: env
const host = `localhost`

type Repo interface {
	ReviewsForGame(gameID, offset, limit int) (goger.RepoReviews, error)
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
	GamesPerDeveloper(limit int) ([]map[string]int64, error)
	Game(title string) (goger.ProductRepo, error)
	MostCommonLanguages(title string, limit uint) ([]goger.LanguageCount, error)
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
	const port = "8080"
	http.HandleFunc("/", handlerIndex)
	http.HandleFunc("/game", handlerGame)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/favicon.ico", faviconHandler)
	log.Printf("Server running on: %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type IndexData struct {
	Trending            []goger.UIGame
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
	GamesPerDeveloper   []map[string]int64
}

type DataGame struct {
	Title       string
	Items       [][2]string // key -> vals
	Languages   []goger.LanguageCount
	Screenshots []string
}

func handlerIndex(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// Get page parameter from the query string
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Check in redis cache for recent response of main page

	b, err := rdb.Get(context.Background(), "index").Bytes()
	if err == nil {
		if _, err = w.Write(b); err != nil {
			log.Printf("Failed to write to response after cache hit: %v", err)
			httpInternal(w)
		}
		// Refresh TTL
		if _, err = rdb.Expire(ctx, "index", time.Minute).Result(); err != nil {
			log.Printf("Failed to refresh cache ttl: %v", err)
			httpInternal(w)
		}
		return
	}

	trendingGames, err := db.MostReviewedGamesWithRevTs(14*24, 5)
	if err != nil {
		log.Printf("Failed to get most reviewed games with rev ts: %v", err)
		httpInternal(w)
	}
	trending := trendingGames.ToUI()

	positiveGames, err := db.MostPopularGames(5)
	if err != nil {
		log.Printf("Failed to get trending games: %v", err)
		httpInternal(w)
		return
	}
	popular := positiveGames.ToUI()

	mostRevsIn1DayGames, err := db.GamesWithMostReviewsIn1Day(5)
	if err != nil {
		log.Printf("Failed to games with most reviews in 1 day: %v", err)
		httpInternal(w)
		return
	}

	positiveGames, err = db.MostPositiveGames(5)
	if err != nil {
		log.Printf("Failed to get most positive games: %v", err)
		httpInternal(w)
		return
	}

	negativeGames, err := db.MostNegativeGames(5)
	if err != nil {
		log.Printf("Failed to get most negative games: %v", err)
		httpInternal(w)
		return
	}

	topLanguages, err := db.TopLanguages(5)
	if err != nil {
		log.Printf("Failed to get top languages: %v", err)
		httpInternal(w)
		return
	}

	topRatingVals, err := db.TopRatingVals(5)
	if err != nil {
		log.Printf("Failed to get top rating val: %v", err)
		httpInternal(w)
		return
	}

	yearsToGames, err := db.TopYearToGamesReleased(100) // all
	if err != nil {
		log.Printf("Failed to get years to games released: %v", err)
		httpInternal(w)
		return
	}

	avgRevsPerUser, err := db.AvgReviewsPerUser()
	if err != nil {
		log.Printf("Failed to get avg reviews per user: %v", err)
		httpInternal(w)
		return
	}

	avgRevsPerGame, err := db.AvgReviewsPerGame()
	if err != nil {
		log.Printf("Failed to get avg reviews per user: %v", err)
		httpInternal(w)
		return
	}

	mostReviewsPerUser, err := db.MostReviewsPerUser()
	if err != nil {
		log.Printf("Failed to get most reviews per user: %v", err)
		httpInternal(w)
		return
	}

	mostReviewsDay, mostReviewsIn1Day, err := db.DayWithMostReviews()
	if err != nil {
		log.Printf("Failed to get day with most reviews: %v", err)
		httpInternal(w)
		return
	}

	gamesPerDev, err := db.GamesPerDeveloper(10)
	if err != nil {
		log.Printf("Failed to get days per developer: %v", err)
		httpInternal(w)
		return
	}

	data := IndexData{
		Trending:            trending,
		Top:                 popular,
		TopGamesIn1Day:      mostRevsIn1DayGames,
		TopPositive:         positiveGames.ToUI(),
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

	var buff bytes.Buffer
	if err = indexTmpl.Execute(&buff, data); err != nil {
		log.Printf("Failed to execute template: %v", err)
		httpInternal(w)
		return
	}

	if _, err = w.Write(buff.Bytes()); err != nil {
		log.Printf("Failed to write response: %v", err)
		httpInternal(w)
		return
	}

	log.Printf("Served games in %v, for IP: %s, URI: %s",
		time.Since(start), r.RemoteAddr, r.RequestURI)
	if err = rdb.Set(ctx, "index", buff.Bytes(), time.Minute).Err(); err != nil {
		log.Printf("Failed to save index page to cache: %v", err)
	}
}

func handlerGame(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	if title == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("title param missing"))
		if err != nil {
			log.Printf("failed to write error response about "+
				"missing title param: %v", err)
		}
		return
	}

	game, err := db.Game(title)
	if err != nil {
		httpInternal(w)
		log.Printf("failed to get game %s: %v", title, err)
		return
	}

	languages, err := db.MostCommonLanguages(title, 5)
	if err != nil {
		httpInternal(w)
		log.Printf("failed to get most common languages: %v", err)
		return
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
	tmpl := template.Must(template.ParseFiles("templates/game.html"))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("failed to execute game template: %v", err)
	}
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/favicon.png")
}

func httpInternal(w http.ResponseWriter) {
	http.Error(w, "Internal error", http.StatusInternalServerError)
}

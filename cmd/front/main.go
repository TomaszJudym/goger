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
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/lib/pq"
	"github.com/tomaszjudym/goger"
)

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
	GamesWithMostReviewsIn1Day(limit int) ([]goger.GameWithMostReviewsIn1Day, error)
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
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/favicon.ico", faviconHandler)
	log.Printf("Server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type IndexData struct {
	Trending       []goger.UIGame
	Top            []goger.UIGame
	TopNegative    []goger.UIGame
	TopPositive    []goger.UIGame
	TopGamesIn1Day []goger.GameWithMostReviewsIn1Day
	TopLanguages   []goger.LanguageCount
}

// TODO: Display on top of page
type Stats struct {
	TotalGames        int
	AvgRatings        float64
	MostPopularGame   string
	MostPositiveGame  string
	MostNegativeGame  string
	AvgReviewsPerGame float64
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

	games, err := db.MostPopularGames(5)
	if err != nil {
		log.Printf("Failed to get trending games: %v", err)
		httpInternal(w)
		return
	}
	popular := games.ToUI()

	mostRevsIn1DayGames, err := db.GamesWithMostReviewsIn1Day(5)
	if err != nil {
		log.Printf("Failed to games with most reviews in 1 day: %v", err)
		httpInternal(w)
		return
	}

	games, err = db.MostPositiveGames(5)
	if err != nil {
		log.Printf("Failed to get most positive games: %v", err)
		httpInternal(w)
		return
	}
	positive := games.ToUI()

	negativeGames, err := db.MostNegativeGames(5)
	if err != nil {
		log.Printf("Failed to get most negative games: %v", err)
		httpInternal(w)
		return
	}
	negative := negativeGames.ToUI()

	topLanguages, err := db.TopLanguages(5)
	if err != nil {
		log.Printf("Failed to get top languages: %v", err)
		httpInternal(w)
	}

	data := IndexData{
		Trending:       trending,
		Top:            popular,
		TopGamesIn1Day: mostRevsIn1DayGames,
		TopPositive:    positive,
		TopNegative:    negative,
		TopLanguages:   topLanguages,
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

	log.Printf("Served games in %v, for IP: %s, URI: %s", time.Since(start), r.RemoteAddr, r.RequestURI)
	if err = rdb.Set(ctx, "index", buff.Bytes(), time.Minute).Err(); err != nil {
		log.Printf("Failed to save index page to cache: %v", err)
	}
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/favicon.png")
}

func httpInternal(w http.ResponseWriter) {
	http.Error(w, "Internal error", http.StatusInternalServerError)
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

const dateLayout = "2006-01-02 15:04:05-07"

type Repo interface {
	GamesWithReviewsCount(offset, limit int) ([]goger.UIGame, error)
	ReviewsForGame(gameID, offset, limit int) (goger.RepoReviews, error)
	CountGames() (int, error)
	GameReviewsCount(gameID string) (int, error)
	MostReviewedGamesWithRevTs(hours, limit int) ([]goger.TrendingGame, error)
	AverageReviewsPerGame() (float64, error)
	MostActiveReviewer() (goger.Reviewer, error)
	MostPositiveReviewer() (string, float64, error)
	MostNegativeReviewer() (string, float64, error)
	MostRecentReview() (goger.Review, error)
	MostPopularGames(limit int) ([]goger.ProductRepo, error)
	GamesWithMostReviewsIn1Day(limit int) ([]goger.GameWithMostReviewsIn1Day, error)
}

type PageGames struct {
	Games      []goger.UIGame
	TotalPages int
}

type ReviewsPage struct {
	Reviews    []goger.UIReview
	TotalPages int
}

const pageSize = 100

var (
	db  Repo
	rdb *redis.Client
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
}

func main() {
	http.HandleFunc("/", handlerIndex)
	http.HandleFunc("/games", handlerGames)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/favicon.ico", faviconHandler)
	const port = "8080"
	log.Printf("Server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type Game struct {
	ID                 string
	Name               string
	Rating             string
	ReleaseDate        string
	TotalReviews       int
	ReviewsPerHoursAgo []int
	Developers         []string
}

type IndexData struct {
	Trending       []Game
	TopGames       []Game
	TopGamesIn1Day []goger.GameWithMostReviewsIn1Day
}

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
		return
	}

	// TODO: Parse once and save
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		httpInternal(w)
		return
	}

	trending, err := db.MostReviewedGamesWithRevTs(14*24, 5)
	if err != nil {
		log.Printf("Failed to get most reviewed games with rev ts: %v", err)
		httpInternal(w)
	}

	trendingGames := make([]Game, len(trending))
	for i, game := range trending {
		trendingGames[i] = Game{
			ID:                 strconv.Itoa(game.ID),
			Name:               game.Title,
			Rating:             strconv.Itoa(game.ReviewsRating),
			TotalReviews:       game.TotalReviews,
			ReviewsPerHoursAgo: daysAgoCount(game.ReviewDates),
		}
	}

	popularGames, err := db.MostPopularGames(5)
	if err != nil {
		log.Printf("Failed to get trending games: %v", err)
		httpInternal(w)
	}

	top := make([]Game, len(popularGames))
	for i, game := range popularGames {
		top[i] = Game{
			ID:           game.ID,
			TotalReviews: game.ReviewsCount,
			Name:         game.Title,
			ReleaseDate:  game.ReleaseDate,
			Rating:       strconv.Itoa(game.ReviewsRating),
			Developers:   game.Developers,
		}
	}

	mostRevsIn1DayGames, err := db.GamesWithMostReviewsIn1Day(5)
	if err != nil {
		log.Printf("Failed to games with most reviews in 1 day: %v", err)
		httpInternal(w)
	}

	data := IndexData{
		Trending:       trendingGames,
		TopGames:       top,
		TopGamesIn1Day: mostRevsIn1DayGames,
	}

	var buff bytes.Buffer
	if err = tmpl.Execute(&buff, data); err != nil {
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
	// Saved page bytes to cache for 1min
	if err = rdb.Set(ctx, "index", buff.Bytes(), time.Minute).Err(); err != nil {
		log.Printf("Failed to save index page to cache: %v", err)
	}
}

func handlerGames(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// Get page parameter from the query string
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	var games []goger.UIGame
	var pagesCount int
	errs := make(chan error, 2)

	go func() {
		var err error
		games, err = getGamesWithRevsCount((page-1)*pageSize, pageSize)
		errs <- err
	}()
	go func() {
		var err error
		gamesCount, err := getGamesCount()
		pagesCount = gamesCount / pageSize
		if page > pagesCount {
			page = 1
		}
		errs <- err
	}()

	for i := 0; i < cap(errs); i++ {
		select {
		case err = <-errs:
			if err != nil {
				log.Printf("Failed to get games: %v", err)
			}
		case <-time.After(10 * time.Second):
			log.Printf("Timeout 10s getting games")
			http.Error(w, "Timeout getting games", http.StatusInternalServerError)
			return
		}
	}

	renderGames(w, PageGames{Games: games, TotalPages: pagesCount})
	log.Printf("Served games in %v, from IP: %s, URI: %s", time.Since(start), r.RemoteAddr, r.RequestURI)
}

func getGamesCount() (int, error) {
	const gamesCountKey = `games:count`
	var gamesCount int
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	val, err := rdb.Get(ctx, gamesCountKey).Result()
	if err == nil {
		gamesCount, err = strconv.Atoi(val)
		if err != nil {
			err = fmt.Errorf("failed to parse games count from cache: %w", err)
		}
	}
	if err != nil {
		if err != redis.Nil {
			log.Printf("Failed to get games count from cache: %v", err)
		}
		var err2 error
		gamesCount, err2 = db.CountGames()
		if err2 != nil {
			return 0, fmt.Errorf("failed to count games from db: %w", err2)
		}
		if err3 := rdb.Set(ctx, gamesCountKey, gamesCount, 10*time.Minute).Err(); err3 != nil {
			log.Printf("Failed to cache games count: %v", err3)
		}
	}
	return gamesCount, nil
}

func getGamesWithRevsCount(offset, limit int) ([]goger.UIGame, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const gamesKey = `games-with-reviews:count:%d:%d`
	val, err := rdb.Get(ctx, fmt.Sprintf(gamesKey, offset, limit)).Result()
	if err == nil {
		var games []goger.UIGame
		if err = json.Unmarshal([]byte(val), &games); err != nil {
			return nil, fmt.Errorf("failed to unmarshal games from cache: %w", err)
		}
		fmt.Println("CACHE HIT for games", offset, limit)
		return games, nil
	}
	games, err := db.GamesWithReviewsCount(offset, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get games from db: %w", err)
	}
	gamesBytes, err := json.Marshal(games)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal games from db: %w", err)
	}
	if err := rdb.Set(ctx, fmt.Sprintf(gamesKey, offset, limit), string(gamesBytes), 10*time.Minute).Err(); err != nil {
		return nil, fmt.Errorf("failed to set games in cache: %w", err)
	}
	return games, nil
}

func renderGames(w http.ResponseWriter, page PageGames) {
	tmpl, err := template.New("games.html").
		Funcs(template.FuncMap{"seq": seq}).
		ParseFiles("templates/games.html")
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		httpInternal(w)
		return
	}

	if err = tmpl.Execute(w, page); err != nil {
		log.Printf("Failed to execute template: %v", err)
		httpInternal(w)
		return
	}
}

func daysAgoCount(dates []string) []int {
	dayCount := make(map[int]int)

	for _, dateStr := range dates {
		t, err := time.Parse(dateLayout, dateStr)
		if err != nil {
			fmt.Println("Error parsing date:", err)
			continue
		}

		daysAgo := int(time.Since(t).Hours() / 24)

		dayCount[daysAgo]++
	}

	maxDaysAgo := 0
	for daysAgo := range dayCount {
		if daysAgo > maxDaysAgo {
			maxDaysAgo = daysAgo
		}
	}

	result := make([]int, maxDaysAgo+1)
	for daysAgo, count := range dayCount {
		result[daysAgo] = count
	}

	return result
}

// TODO: Finish pagination. Display only +/- 30 (?) numbers
func seq(n int) []int {
	result := make([]int, n)
	for i := range result {
		result[i] = i + 1
	}
	return result
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/favicon.png")
}

func httpInternal(w http.ResponseWriter) {
	http.Error(w, "Internal error", http.StatusInternalServerError)
}

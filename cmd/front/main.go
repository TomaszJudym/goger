package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/tomaszjudym/goger"
)

type Repo interface {
	GamesWithReviewsCount(offset, limit int) ([]goger.UIGame, error)
	ReviewsForGame(gameID, offset, limit int) (goger.RepoReviews, error)
	CountGames() (int, error)
	GameReviewsCount(gameID string) (int, error)
}

type PageGames struct {
	Games      []goger.UIGame
	TotalPages int
}

type ReviewsPage struct {
	Reviews    []goger.UIReview
	TotalPages int
}

var (
	db Repo
)

func init() {
	var err error
	db, err = goger.NewRepo()
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}
}

func main() {
	http.HandleFunc("/games", handlerGames)
	http.HandleFunc("/games/", handlerReviews)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/favicon.ico", faviconHandler)
	const port = "8080"
	log.Printf("Server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handlerGames(w http.ResponseWriter, r *http.Request) {
	// Get page parameter from the query string
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	start := time.Now()
	gamesCount, err := db.CountGames()
	took := time.Since(start)
	if err != nil {
		log.Printf("Failed to count games: %v", err)
		http.Error(w, "Internal server Error", http.StatusInternalServerError)
		return
	}
	pagesCount := gamesCount / 50

	if page > pagesCount {
		page = 1
	}

	// Fetch games with reviews count from the database
	start = time.Now()
	games, err := db.GamesWithReviewsCount((page-1)*50, 50)
	if err != nil {
		log.Printf("Failed to cout games reviews offset: %d: %v", (page-1)*50, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	took = time.Since(start)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	var revs int
	for _, g := range games {
		revs += g.ReviewsCount
	}
	log.Printf("Fetched page: %d of: %d games with: %d reviews in in: %v",
		page, gamesCount, revs, took)

	renderGames(w, PageGames{Games: games, TotalPages: pagesCount})
}

func handlerReviews(w http.ResponseWriter, r *http.Request) {
	// Get page parameter from the query string
	segments := strings.Split(r.URL.Path, "/")
	l := len(segments)
	if l != 4 {
		http.Error(w, fmt.Sprintf("Path should have 4 elements but got %d",
			l), http.StatusBadRequest)
		log.Printf("Want 4 path elems got: %d in %v", l, segments)
		return
	}

	gameID := segments[2]
	id, err := strconv.Atoi(gameID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid game ID: %s", gameID), http.StatusBadRequest)
		log.Printf("Failed to convert gameID: %s to int: %v", gameID, err)
		return
	}

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	start := time.Now()
	reviews, err := db.ReviewsForGame(id, (page-1)*50, 50)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		log.Printf("Failed to get page: %d reviews for game: %s, err: %v", page, gameID, err)
		return
	}
	log.Printf("Fetched: %d reviews for: %d in: %v", len(reviews), id, time.Since(start))
	// TODO: Fix - fetches nothing
	start = time.Now()
	count, err := db.GameReviewsCount(gameID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		log.Printf("Failed to count reviews of game: %s, err: %v", gameID, err)
		return
	}
	log.Printf("Counted: %d reviews in: %v", count, time.Since(start))

	renderReviews(w, ReviewsPage{Reviews: reviews.ToUI(), TotalPages: count / 50})
}

func renderGames(w http.ResponseWriter, page PageGames) {
	tmpl, err := template.New("index.html").
		Funcs(template.FuncMap{"seq": seq}).
		ParseFiles("templates/index.html")
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err = tmpl.Execute(w, page); err != nil {
		log.Printf("Failed to execute template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func renderReviews(w http.ResponseWriter, page ReviewsPage) {
	tmpl, err := template.New("reviews.html").
		Funcs(template.FuncMap{"seq": seq}).
		ParseFiles("templates/reviews.html")
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err = tmpl.Execute(w, page); err != nil {
		log.Printf("Failed to execute template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
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

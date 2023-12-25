package main

import (
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	"github.com/tomaszjudym/goger"
)

type Repo interface {
	GamesWithReviewsCount(offset, limit int) ([]goger.UIGame, error)
	CountGames() (int, error)
}

// Page represents the data to be rendered on the webpage.
type Page struct {
	Games      []goger.UIGame
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
	http.HandleFunc("/games", handler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/favicon.ico", faviconHandler)
	const port = "8080"
	log.Printf("Server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
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

	renderTemplate(w, Page{Games: games, TotalPages: pagesCount})
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/favicon.png")
}

// TODO: Finish pagination. Display only +/- 30 (?) numbers
func seq(n int) []int {
	result := make([]int, n)
	for i := range result {
		result[i] = i + 1
	}
	return result
}

func renderTemplate(w http.ResponseWriter, page Page) {
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

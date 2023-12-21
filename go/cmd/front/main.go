package main

import (
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/tomaszjudym/goger"
)

// Game represents the structure of a game.
type Game struct {
	ID           int
	Title        string
	ReviewsCount int
}

// Page represents the data to be rendered on the webpage.
type Page struct {
	Games      []Game
	TotalPages int
}

var (
	db *sqlx.DB
)

func init() {
	var err error
	db, err = goger.ConnectDB()
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}
}

func main() {
	http.HandleFunc("/", handler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("chuj"))
		if err != nil {
			log.Printf("ERROR: FAILED TO WRITE CHUJ: %v", err)
		}
	})

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

	gamesCount, err := goger.CountGames(db)
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
	games, err := getGamesWithReviews((page-1)*50, 50)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Render the HTML template
	renderTemplate(w, Page{Games: games, TotalPages: pagesCount})
}

func getGamesWithReviews(offset, limit int) ([]Game, error) {
	rows, err := db.Query(`
		SELECT g.id, g.title, COUNT(r.id) AS reviews_count
		FROM games g
		LEFT JOIN reviews r ON g.id = r.product_id
		GROUP BY g.id, g.title
		ORDER BY reviews_count DESC, g.id
		OFFSET $1
		LIMIT $2
	`, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []Game
	for rows.Next() {
		var game Game
		err := rows.Scan(&game.ID, &game.Title, &game.ReviewsCount)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	return games, nil
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

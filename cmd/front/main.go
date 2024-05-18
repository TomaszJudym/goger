package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/render"
	"github.com/go-redis/redis/v8"
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

const pageSize = 100

var (
	db  Repo
	rdb *redis.Client
)

func init() {
	var err error
	db, err = goger.NewRepo()
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
	http.HandleFunc("/games/", handlerReviews)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/favicon.ico", faviconHandler)
	const port = "8080"
	log.Printf("Server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type Game struct {
	ID             string
	Name           string
	Change         string
	CurrentPlayers int
	PeakPlayers    int
	HoursPlayed    int
	Time           string
	Data           []int
}

type ChartData struct {
	ID     string   `json:"id"`
	Labels []string `json:"labels"`
	Values []int    `json:"values"`
}

type IndexData struct {
	Trending   []Game
	TopGames   []Game
	TopRecords []Game
}

// adapted from
// https://github.com/go-echarts/go-echarts/blob/master/templates/base.go
// https://github.com/go-echarts/go-echarts/blob/master/templates/header.go
var baseTpl = `
<script type="text/javascript">
    "use strict";
    let goecharts_{{ .ChartID | safeJS }} = echarts.init(document.getElementById('{{ .ChartID | safeJS }}'));
    let option_{{ .ChartID | safeJS }} = {{ .JSON }};
    goecharts_{{ .ChartID | safeJS }}.setOption(option_{{ .ChartID | safeJS }});
</script>
`

type snippetRenderer struct {
	c      any
	before []func()
}

func newSnippetRenderer(c interface{}, before ...func()) render.Renderer {
	return &snippetRenderer{c: c, before: before}
}

func (r *snippetRenderer) Render(w io.Writer) error {
	const tplName = "chart"

	tpl := template.
		Must(template.New(tplName).
			Funcs(template.FuncMap{
				"safeJS": func(s interface{}) template.JS {
					return template.JS(fmt.Sprint(s))
				},
			}).
			Parse(baseTpl),
		)

	err := tpl.ExecuteTemplate(w, tplName, r.c)
	return err
}

func renderToHTML(r render.Renderer) template.HTML {
	var buf bytes.Buffer
	err := r.Render(&buf)
	if err != nil {
		log.Printf("Failed to render chart: %s", err)
		return ""
	}

	return template.HTML(buf.String())
}

func generateBarItems() []opts.BarData {
	items := make([]opts.BarData, 0)
	for i := 0; i < 10; i++ {
		items = append(items, opts.BarData{Value: rand.Intn(300)})
	}
	return items
}

func randomIntArray(size int) []int {
	result := make([]int, size)
	for i := range result {
		result[i] = rand.Intn(100)
	}
	return result
}

func randomDate() string {
	min := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	max := time.Now()
	return min.Add(time.Duration(rand.Int63n(int64(max.Sub(min))))).Truncate(time.Hour).String()
}
func handlerIndex(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// Get page parameter from the query string
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	// TODO: Parse once and save
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	trendingGames := []Game{
		{"1", "480% Orange Juice", "+1442.6%", 1493, 0, 0, randomDate(), randomIntArray(48)},
		{"2", "ENDLESS™ Legend", "+946.9%", 2306, 0, 0, randomDate(), randomIntArray(48)},
		{"3", "Wizard with a Gun", "+425.5%", 1140, 0, 0, randomDate(), randomIntArray(48)},
		{"4", "Minecraft Dungeons", "+277.0%", 1748, 0, 0, randomDate(), randomIntArray(48)},
		{"5", "Wildermyth", "+242.1%", 1347, 0, 0, randomDate(), randomIntArray(48)},
	}

	data := IndexData{
		Trending: trendingGames,
		TopGames: []Game{
			{"6", "Counter-Strike 2", "", 809646, 1614925, 683750246, randomDate(), randomIntArray(48)},
			{"7", "Dota 2", "", 396802, 921133, 362647668, randomDate(), randomIntArray(48)},
			{"8", "PUBG: BATTLEGROUNDS", "", 111874, 693485, 200697994, randomDate(), randomIntArray(48)},
			{"9", "Rust", "", 90420, 153662, 59446243, randomDate(), randomIntArray(48)},
			{"48", "Apex Legends", "", 89447, 430800, 126331197, randomDate(), randomIntArray(48)},
			{"11", "Call of Duty®", "", 84016, 134190, 56815113, randomDate(), randomIntArray(48)},
			{"12", "Fallout 4", "", 80504, 186746, 67500716, randomDate(), randomIntArray(48)},
			{"13", "Destiny 2", "", 79624, 125545, 49370136, randomDate(), randomIntArray(48)},
			{"14", "Grand Theft Auto V", "", 73194, 158791, 66264374, randomDate(), randomIntArray(48)},
			{"15", "Team Fortress 2", "", 72470, 84858, 47007870, randomDate(), randomIntArray(48)},
		},
		TopRecords: []Game{
			{"16", "PUBG: BATTLEGROUNDS", "3,236,027", 0, 0, 0, "Jan 2018", randomIntArray(48)},
			{"17", "Palworld", "2,481,535", 0, 0, 0, "Jan 2024", randomIntArray(48)},
			{"18", "Counter-Strike 2", "1,802,853", 0, 0, 0, "May 2023", randomIntArray(48)},
			{"19", "Lost Ark", "1,324,761", 0, 0, 0, "Feb 2022", randomIntArray(48)},
			{"20", "Dota 2", "1,291,328", 0, 0, 0, "Mar 2016", randomIntArray(48)},
			{"21", "ELDEN RING", "952,523", 0, 0, 0, "Mar 2022", randomIntArray(48)},
			{"22", "New World", "913,027", 0, 0, 0, "Oct 2021", randomIntArray(48)},
			{"23", "Baldur's Gate 3", "875,343", 0, 0, 0, "Aug 2023", randomIntArray(48)},
		},
	}

	if err = tmpl.Execute(w, data); err != nil {
		log.Printf("Failed to execute template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("Served games in %v, from IP: %s, URI: %s", time.Since(start), r.RemoteAddr, r.RequestURI)
}

// TODO: make this bar work somehow
func randomBarHTML() template.HTML {
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Width: "100px", Height: "200px"}),
		charts.WithLegendOpts(opts.Legend{Show: false}),
		charts.WithXAxisOpts(opts.XAxis{Show: false}),
		charts.WithYAxisOpts(opts.YAxis{Show: false}),
	)

	bar.AddSeries("", generateBarItems()).
		AddSeries("", generateBarItems())
	return renderToHTML(bar)
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

func handlerReviews(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// Get page parameter from the query string
	segments := strings.Split(r.URL.Path, "/")
	l := len(segments)
	if l != 4 {
		msg := fmt.Sprintf("Want 4 path elems got: %d in %v", l, segments)
		http.Error(w, msg, http.StatusBadRequest)
		log.Println(msg)
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
	var reviews goger.RepoReviews
	var count int
	reviewsKey := fmt.Sprintf("reviews:%d:page:%d", id, page)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reviewsJson, err := rdb.Get(ctx, reviewsKey).Result()
	if err == redis.Nil {
		// Cache miss, fetch from DB and cache it
		errs := make(chan error, 2)

		go func() {
			var err error
			reviews, err = db.ReviewsForGame(id, (page-1)*50, 50)
			if err != nil {
				errs <- fmt.Errorf("failed to get page: %d reviews for game: %s, err: %v", page, gameID, err)
				return
			}
			errs <- nil
		}()

		go func() {
			var err error
			count, err = db.GameReviewsCount(gameID)
			if err != nil {
				errs <- fmt.Errorf("failed to count reviews of game: %s, err: %v", gameID, err)
				return
			}
			errs <- nil
		}()

		for i := 0; i < 2; i++ {
			err := <-errs
			if err != nil {
				http.Error(w, "Internal error", http.StatusInternalServerError)
				log.Print(err)
				return
			}
		}
		reviewsJson, err := json.Marshal(reviews)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			log.Println("Failed to unmarshal reviews:", err)
			return
		}
		rdb.Set(ctx, reviewsKey, reviewsJson, 30*time.Minute) // Adjust TTL as needed
	} else if err != nil {
		log.Printf("Error getting reviews from cache: %v", err)
		// Handle error
	} else {
		// Cache hit, deserialize JSON to reviews
		if err = json.Unmarshal([]byte(reviewsJson), &reviews); err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			log.Println("Failed to unmarshal reviews from cache:", err)
			return
		}
	}

	renderReviews(w, ReviewsPage{Reviews: reviews.ToUI(), TotalPages: count / 50})
	log.Printf("Served %d reviews in: %v IP: %s URI: %s", len(reviews), time.Since(start), r.RemoteAddr, r.RequestURI)
}

func renderGames(w http.ResponseWriter, page PageGames) {
	tmpl, err := template.New("games.html").
		Funcs(template.FuncMap{"seq": seq}).
		ParseFiles("templates/games.html")
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

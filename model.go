package goger

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

const emptyDate = `0001-01-01`

type Reviews []Review
type Products []Product
type ProductsRepo []ProductRepo
type TrendingGames []TrendingGame

func (r Reviews) ToRepo(ts time.Time) []ReviewRepo {
	ret := make([]ReviewRepo, 0, len(r))
	for _, rev := range r {
		ret = append(ret, rev.toRepo(ts))
	}
	return ret
}

func (p Products) ToRepo(ts time.Time) ([]ProductRepo, error) {
	ret := make([]ProductRepo, 0, len(p))
	for _, prod := range p {
		repoProd, err := prod.ToRepo(ts)
		if err != nil {
			return nil, fmt.Errorf("failed to convert: %s to repo: %w",
				prod.Title, err)
		}
		ret = append(ret, repoProd)
	}
	return ret, nil
}

type CatalogResp struct {
	Pages        int      `json:"pages"`
	ProductCount int      `json:"productCount"`
	Products     Products `json:"products"`
}

func (c CatalogResp) GameIDsToTitles() map[string]string {
	ret := make(map[string]string, len(c.Products))
	for _, p := range c.Products {
		ret[p.ID] = p.Title
	}
	return ret
}

type Product struct {
	ID                    string         `json:"id"`
	Slug                  string         `json:"slug"`
	Features              []nameSlugPair `json:"features"`
	Screenshots           []string       `json:"screenshots"`
	UserPreferredLanguage struct {
		Code    string `json:"code"`
		InAudio bool   `json:"inAudio"`
		InText  bool   `json:"inText"`
	} `json:"userPreferredLanguage"`
	ReleaseDate      string   `json:"releaseDate"`
	StoreReleaseDate string   `json:"storeReleaseDate"`
	ProductType      string   `json:"productType"`
	Title            string   `json:"title"`
	CoverHorizontal  string   `json:"coverHorizontal"`
	CoverVertical    string   `json:"coverVertical"`
	Developers       []string `json:"developers"`
	Publishers       []string `json:"publishers"`
	OperatingSystems []string `json:"operatingSystems"`
	Price            struct {
		Final      string `json:"final"`
		Base       string `json:"base"`
		Discount   any    `json:"discount"`
		FinalMoney struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
			Discount string `json:"discount"`
		} `json:"finalMoney"`
		BaseMoney struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"baseMoney"`
	} `json:"price"`
	ProductState  string         `json:"productState"`
	Genres        []nameSlugPair `json:"genres"`
	Tags          []nameSlugPair `json:"tags"`
	ReviewsRating int            `json:"reviewsRating"`
}

type nameSlugPair struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ProductRepo represents the simplified representation for PostgreSQL storage
type ProductRepo struct {
	ID                       string         `db:"id" json:"id"`
	Slug                     string         `db:"slug" json:"slug"`
	Features                 pq.StringArray `db:"features" json:"features"`
	Screenshots              pq.StringArray `db:"screenshots" json:"screenshots"`
	UserPreferredLangCode    string         `db:"user_preferred_language_code" json:"userPreferredLanguage,omitempty"`
	UserPreferredLangInAudio bool           `db:"user_preferred_language_in_audio" json:"userPreferredLanguageInAudio,omitempty"`
	UserPreferredLangInText  bool           `db:"user_preferred_language_in_text" json:"userPreferredLanguageInText,omitempty"`
	ReleaseDate              string         `db:"release_date" json:"releaseDate"`
	StoreReleaseDate         string         `db:"store_release_date" json:"storeReleaseDate"`
	ProductType              string         `db:"product_type" json:"productType"`
	Title                    string         `db:"title" json:"title"`
	CoverHorizontal          string         `db:"cover_horizontal" json:"coverHorizontal"`
	CoverVertical            string         `db:"cover_vertical" json:"coverVertical"`
	Developers               pq.StringArray `db:"developers" json:"developers"`
	Publishers               pq.StringArray `db:"publishers" json:"publishers"`
	OperatingSystems         pq.StringArray `db:"operating_systems" json:"operatingSystems"`
	PriceFinal               float64        `db:"price_final" json:"price,omitempty"`
	PriceBase                float64        `db:"price_base" json:"priceBase,omitempty"`
	PriceCurrency            string         `db:"price_currency" json:"priceCurrency,omitempty"`
	PriceDiscount            float64        `db:"price_discount" json:"priceDiscount,omitempty"`
	ProductState             string         `db:"product_state" json:"productState"`
	Genres                   pq.StringArray `db:"genres" json:"genres"`
	Tags                     pq.StringArray `db:"tags" json:"tags"`
	ReviewsCount             int            `db:"reviews_count" json:"reviewsCount"`
	ReviewsRating            int            `db:"reviews_rating" json:"reviewsRating"`
	UpdatedAt                []byte         `db:"updated_at" json:"-"`
}

// UnmarshalJSON is a custom unmarshaller for ProductRepo which handles ID wether it's string or int
func (r *ProductRepo) UnmarshalJSON(b []byte) error {
	type alias ProductRepo

	aux := &struct {
		ID any `json:"id"`
		*alias
	}{
		ID:    0,
		alias: &alias{},
	}

	// TODO: Fix this unmarshalling
	if err := json.Unmarshal(b, &aux); err != nil {
		return fmt.Errorf("UNMARSHAL FAILED XD: %w", err)
	}

	switch v := aux.ID.(type) {
	case int:
		r.ID = fmt.Sprintf("%d", v)
	case string:
		r.ID = v
	case float64:
		r.ID = fmt.Sprintf("%f", v)
	default:
		return fmt.Errorf("unsupported ID type: %T", v)
	}

	*r = ProductRepo(*aux.alias)

	return nil
}

func (p *ProductsRepo) ToUI() []UIGame {
	ret := make([]UIGame, 0, len(*p))
	for _, prod := range *p {
		ret = append(ret, prod.ToUI())
	}
	return ret
}

func (p *ProductRepo) ToUI() UIGame {
	return UIGame{
		ID:           p.ID,
		Name:         p.Title,
		Rating:       p.ReviewsRating,
		ReleaseDate:  p.ReleaseDate,
		TotalReviews: p.ReviewsCount,
		Developers:   p.Developers,
	}
}

// MapProductToRepo maps the original Product to the simplified ProductRepo
func (p Product) ToRepo(ts time.Time) (ProductRepo, error) {
	// Prices can arrive empty for some reason
	if p.Price.Final == "" {
		p.Price.Final = "0.0"
	}
	if p.Price.Base == "" {
		p.Price.Base = "0.0"
	}
	if p.Price.FinalMoney.Discount == "" {
		p.Price.FinalMoney.Discount = "0.0"
	}
	// Prices can have currency ($) string in them.
	// Extract pure float from them.
	priceFinal, err := extractFloat(p.Price.Final)
	if err != nil {
		return ProductRepo{}, fmt.Errorf("failed to extract float "+
			"from final price: %s: %w", p.Price.Final, err)
	}
	priceBase, err := extractFloat(p.Price.Base)
	if err != nil {
		return ProductRepo{}, fmt.Errorf("failed to extract float "+
			"from base price: %s: %w", p.Price.Base, err)
	}
	priceDiscount, err := extractFloat(p.Price.FinalMoney.Discount)
	if err != nil {
		return ProductRepo{}, fmt.Errorf("failed to extract float "+
			"from discount: %s: %w", p.Price.FinalMoney.Discount, err)
	}
	// Dates can be empty. If only one is present (like with cyberpunk 2077)
	// set both dates to it. Otherwise just set zero value or db will cry about
	// empty string date.
	if p.ReleaseDate == "" && p.StoreReleaseDate == "" {
		p.ReleaseDate, p.StoreReleaseDate = emptyDate, emptyDate
	}
	if p.StoreReleaseDate == "" {
		p.StoreReleaseDate = p.ReleaseDate
	}
	if p.ReleaseDate == "" {
		p.ReleaseDate = p.StoreReleaseDate
	}
	// Screenshots contain placeholder {formatter} instead of exact URL.
	// On cyberpunk phantom liberty page it "product_card_v2_mobile_slider_639".
	// It's working so apply this to placeholder in strings.
	for i, s := range p.Screenshots {
		const rep = "product_card_v2_mobile_slider_639"
		p.Screenshots[i] = strings.Replace(s, "{formatter}", rep, 1)
	}

	return ProductRepo{
		ID:                       p.ID,
		Slug:                     p.Slug,
		Features:                 extractSlugs(p.Features),
		Screenshots:              p.Screenshots,
		UserPreferredLangCode:    p.UserPreferredLanguage.Code,
		UserPreferredLangInAudio: p.UserPreferredLanguage.InAudio,
		UserPreferredLangInText:  p.UserPreferredLanguage.InText,
		ReleaseDate:              p.ReleaseDate,
		StoreReleaseDate:         p.StoreReleaseDate,
		ProductType:              p.ProductType,
		Title:                    p.Title,
		CoverHorizontal:          p.CoverHorizontal,
		CoverVertical:            p.CoverVertical,
		Developers:               p.Developers,
		Publishers:               p.Publishers,
		OperatingSystems:         p.OperatingSystems,
		PriceFinal:               priceFinal,
		PriceBase:                priceBase,
		PriceCurrency:            p.Price.FinalMoney.Currency,
		PriceDiscount:            priceDiscount,
		ProductState:             p.ProductState,
		Genres:                   extractSlugs(p.Genres),
		Tags:                     extractSlugs(p.Tags),
		ReviewsRating:            p.ReviewsRating,
		UpdatedAt:                pq.FormatTimestamp(ts),
	}, nil
}

func extractFloat(input string) (float64, error) {
	// Define a regular expression to match floating-point numbers
	re := regexp.MustCompile(`[-]?\d+(\.\d+)?`)

	// Find the first match in the input string
	match := re.FindString(input)

	// Convert the matched string to a floating-point number
	result, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0, err
	}

	return result, nil
}

// extractSlugs returns only slugs from passed pairs.
func extractSlugs(pairs []nameSlugPair) []string {
	slugs := make([]string, 0, len(pairs))
	for _, v := range pairs {
		slugs = append(slugs, v.Slug)
	}
	return slugs
}

// #################################################
// Reviews
// #################################################

type ReviewsResp struct {
	Page              int             `json:"page"`
	Limit             int             `json:"limit"`
	Pages             int             `json:"pages"`
	ReviewCount       int             `json:"reviewCount"`
	OverallAvgRating  float64         `json:"overallAvgRating"`
	FilteredAvgRating float64         `json:"filteredAvgRating"`
	MostHelpful       MostHelpful     `json:"mostHelpful"`
	IsReviewable      bool            `json:"isReviewable"`
	Links             NavigationLinks `json:"_links"`
	Embedded          Embedded        `json:"_embedded"`
}

type Rating struct {
	Value int `json:"value"`
}

type Content struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Language    string `json:"language"`
}

type AvatarLinks struct {
	GogImageID string `json:"gog_image_id"`
	Small      string `json:"small"`
	Small2X    string `json:"small_2x"`
	Medium     string `json:"medium"`
	Medium2X   string `json:"medium_2x"`
	Large      string `json:"large"`
	Large2X    string `json:"large_2x"`
	SdkImg32   string `json:"sdk_img_32"`
	SdkImg64   string `json:"sdk_img_64"`
	SdkImg184  string `json:"sdk_img_184"`
	MenuSmall  string `json:"menu_small"`
	MenuSmall2 string `json:"menu_small_2"`
	MenuBig    string `json:"menu_big"`
	MenuBig2   string `json:"menu_big_2"`
}

type Avatar struct {
	Links any `json:"links"`
}
type Counters struct {
	Games   int `json:"games"`
	Reviews int `json:"reviews"`
}
type Reviewer struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Avatar   Avatar   `json:"avatar"`
	Counters Counters `json:"counters"`
}
type Votes struct {
	Downvotes int `json:"downvotes"`
	Upvotes   int `json:"upvotes"`
}
type Vote struct {
	Href string `json:"href"`
}
type Report struct {
	Href string `json:"href"`
}
type Links struct {
	Vote   Vote   `json:"vote"`
	Report Report `json:"report"`
}

type MostHelpful struct {
	ID                 string    `json:"id"`
	ProductID          string    `json:"productId"`
	Rating             Rating    `json:"rating"`
	Content            Content   `json:"content"`
	Reviewer           Reviewer  `json:"reviewer"`
	Labels             []string  `json:"labels"`
	Votes              Votes     `json:"votes"`
	Date               time.Time `json:"date"`
	CreationDate       time.Time `json:"creationDate"`
	InternalUpdateDate string    `json:"internalUpdateDate"`
}

type HrefWrapper struct {
	Href string `json:"href"`
}

type NavigationLinks struct {
	First    HrefWrapper `json:"first"`
	Last     HrefWrapper `json:"last"`
	Next     HrefWrapper `json:"next"`
	Previous HrefWrapper `json:"previous"`
}

type Review struct {
	ID                 string   `json:"id"`
	ProductID          string   `json:"productId"`
	Rating             Rating   `json:"rating"`
	Content            Content  `json:"content"`
	Reviewer           Reviewer `json:"reviewer"`
	Labels             []string `json:"labels"`
	Votes              Votes    `json:"votes"`
	Date               string   `json:"date"`
	CreationDate       string   `json:"creationDate"`
	InternalUpdateDate string   `json:"internalUpdateDate"`
}

type Embedded struct {
	Reviews Reviews `json:"items"`
}

func (r Review) toRepo(ts time.Time) ReviewRepo {
	links, err := toRepoLinks(r.Reviewer.Avatar.Links)
	if err != nil {
		log.Printf("WARN: Failed to convert links to repo: %v\n", err)
	}
	if r.Date == "" {
		r.Date = emptyDate
	}
	if r.CreationDate == "" {
		r.CreationDate = emptyDate
	}
	if r.InternalUpdateDate == "" {
		r.InternalUpdateDate = emptyDate
	}
	prodID, err := strconv.Atoi(r.ProductID)
	if err != nil {
		log.Printf("WARN: Failed to convert product_id of review: %s: "+
			"%s to int: %v", r.ID, r.ProductID, err)
	}
	return ReviewRepo{
		ID:                 r.ID,
		ProductID:          prodID,
		RatingValue:        r.Rating.Value,
		Title:              r.Content.Title,
		Description:        r.Content.Description,
		Language:           r.Content.Language,
		ReviewerID:         r.Reviewer.ID,
		ReviewerUsername:   r.Reviewer.Username,
		AvatarGogImageID:   links.GogImageID,
		AvatarLarge:        links.Large,
		AvatarSDKImg184:    links.SdkImg184,
		AvatarMenuBig:      links.MenuBig,
		CountersGames:      r.Reviewer.Counters.Games,
		CountersReviews:    r.Reviewer.Counters.Reviews,
		Labels:             pq.StringArray(r.Labels),
		Downvotes:          r.Votes.Downvotes,
		Upvotes:            r.Votes.Upvotes,
		ReviewDate:         r.Date,
		CreationDate:       r.CreationDate,
		InternalUpdateDate: r.InternalUpdateDate,
		UpdatedAt:          pq.FormatTimestamp(ts),
	}
}

// toRepoLinks converts expecter in payload links to
// their repo representation
func toRepoLinks(respLinks any) (AvatarLinks, error) {
	var links AvatarLinks
	l := respLinks
	if l != nil {
		var ok bool
		links, ok = l.(AvatarLinks)
		if !ok {
			// Otherwise it can present as map[string]any
			m, ok := l.(map[string]any)
			if ok {
				b, err := json.Marshal(m)
				if err != nil {
					return links, fmt.Errorf("WARN: Failed to "+
						"marshal links: %v (%T): %v", m, m, err)
				}

				if err = json.Unmarshal(b, &links); err != nil {
					return links, fmt.Errorf("WARN: Failed to unmarshal "+
						"links: %v (%T) into AvatarLinks: %v", m, m, err)
				}
			}
		}
	}
	return links, nil
}

type ReviewRepo struct {
	ID                 string         `db:"id" json:"id"`
	ProductID          int            `db:"product_id" json:"productId"`
	RatingValue        int            `db:"rating_value" json:"ratingValue"`
	Title              string         `db:"title" json:"title"`
	Description        string         `db:"description" json:"description"`
	Language           string         `db:"language" json:"language"`
	ReviewerID         string         `db:"reviewer_id" json:"reviewerId"`
	ReviewerUsername   string         `db:"reviewer_username" json:"reviewerUsername"`
	AvatarGogImageID   string         `db:"avatar_gog_image_id" json:"avatarGogImageId"`
	AvatarLarge        string         `db:"avatar_large" json:"avatarLarge"`
	AvatarSDKImg184    string         `db:"avatar_sdk_img_184" json:"avatarSdkImg184"`
	AvatarMenuBig      string         `db:"avatar_menu_big" json:"avatarMenuBig"`
	CountersGames      int            `db:"counters_games" json:"countersGames"`
	CountersReviews    int            `db:"counters_reviews" json:"countersReviews"`
	Labels             pq.StringArray `db:"labels" json:"labels"`
	Downvotes          int            `db:"downvotes" json:"downvotes"`
	Upvotes            int            `db:"upvotes" json:"upvotes"`
	ReviewDate         string         `db:"review_date" json:"reviewDate"`
	CreationDate       string         `db:"creation_date" json:"creationDate"`
	InternalUpdateDate string         `db:"internal_update_date" json:"internalUpdateDate"`
	UpdatedAt          []byte         `db:"updated_at" json:"updatedAt"`
}

//nolint:gocyclo
func (r *ReviewRepo) UnmarshalJSON(b []byte) error {
	type alias ReviewRepo

	aux := &struct {
		ID any `json:"id"`
		*alias
	}{
		ID:    0,
		alias: &alias{},
	}
	// TODO: Fix this unmarshalling
	if err := json.Unmarshal(b, &aux); err != nil {
		return fmt.Errorf("UNMARSHAL FAILED XD: %w", err)
	}

	*r = ReviewRepo(*aux.alias)

	return nil
}

func (r ReviewRepo) ToUI() UIReview {
	return UIReview{
		ProductID:          r.ProductID,
		RatingValue:        r.RatingValue,
		Title:              r.Title,
		Description:        r.Description,
		Language:           r.Language,
		ReviewerUsername:   r.ReviewerUsername,
		CountersGames:      r.CountersGames,
		CountersReviews:    r.CountersReviews,
		Labels:             r.Labels,
		Downvotes:          r.Downvotes,
		Upvotes:            r.Upvotes,
		ReviewDate:         r.ReviewDate,
		CreationDate:       r.CreationDate,
		InternalUpdateDate: r.InternalUpdateDate,
	}
}

type RepoReviews []ReviewRepo

func (u RepoReviews) ToUI() []UIReview {
	ret := make([]UIReview, 0, len(u))
	for _, r := range u {
		ret = append(ret, r.ToUI())
	}
	return ret
}

type UIReview struct {
	ProductID          int
	RatingValue        int
	Title              string
	Description        string
	Language           string
	ReviewerUsername   string
	CountersGames      int
	CountersReviews    int
	Labels             []string
	Downvotes          int
	Upvotes            int
	ReviewDate         string
	CreationDate       string
	InternalUpdateDate string
}

type TrendingGame struct {
	ID            int            `db:"id"`
	Title         string         `db:"title"`
	ReviewsRating int            `db:"reviews_rating"`
	TotalReviews  int            `db:"total_reviews"`
	ReviewDates   pq.StringArray `db:"review_dates"`
}

func (tr TrendingGames) ToUI() []UIGame {
	ret := make([]UIGame, 0, len(tr))
	for _, game := range tr {
		ret = append(ret, UIGame{
			ID:                 strconv.Itoa(game.ID),
			Name:               game.Title,
			Rating:             game.ReviewsRating,
			TotalReviews:       game.TotalReviews,
			ReviewsPerHoursAgo: daysAgoCount(game.ReviewDates),
		})
	}
	return ret
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

type GameWithMostReviewsIn1Day struct {
	Title        string  `db:"title"`
	ReleaseDate  string  `db:"release_date"`
	ReviewDate   string  `db:"review_date"`
	TotalReviews int     `db:"total_reviews"`
	Rating       float64 `db:"rating"`
}

type UIGame struct {
	ID                 string
	Name               string
	Rating             int
	ReleaseDate        string
	TotalReviews       int
	ReviewsPerHoursAgo []int
	Developers         []string
}

type RunRepo struct {
	Games   int       `db:"games"`
	Pages   int       `db:"pages"`
	StartTs time.Time `db:"start_ts"`
	EndTs   time.Time `db:"end_ts"`
}

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

func (r Reviews) ToRepo() []ReviewRepo {
	ret := make([]ReviewRepo, 0, len(r))
	for _, rev := range r {
		ret = append(ret, rev.toRepo())
	}
	return ret
}

func (p Products) ToRepo() ([]ProductRepo, error) {
	ret := make([]ProductRepo, 0, len(p))
	for _, prod := range p {
		repoProd, err := prod.ToRepo()
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
	ID                       string         `db:"id"`
	Slug                     string         `db:"slug"`
	Features                 pq.StringArray `db:"features"`
	Screenshots              pq.StringArray `db:"screenshots"`
	UserPreferredLangCode    string         `db:"user_preferred_language_code"`
	UserPreferredLangInAudio bool           `db:"user_preferred_language_in_audio"`
	UserPreferredLangInText  bool           `db:"user_preferred_language_in_text"`
	ReleaseDate              string         `db:"release_date"`
	StoreReleaseDate         string         `db:"store_release_date"`
	ProductType              string         `db:"product_type"`
	Title                    string         `db:"title"`
	CoverHorizontal          string         `db:"cover_horizontal"`
	CoverVertical            string         `db:"cover_vertical"`
	Developers               pq.StringArray `db:"developers"`
	Publishers               pq.StringArray `db:"publishers"`
	OperatingSystems         pq.StringArray `db:"operating_systems"`
	PriceFinal               float64        `db:"price_final"`
	PriceBase                float64        `db:"price_base"`
	PriceCurrency            string         `db:"price_currency"`
	PriceDiscount            float64        `db:"price_discount"`
	ProductState             string         `db:"product_state"`
	Genres                   pq.StringArray `db:"genres"`
	Tags                     pq.StringArray `db:"tags"`
	ReviewsRating            int            `db:"reviews_rating"`
}

// MapProductToRepo maps the original Product to the simplified ProductRepo
func (p Product) ToRepo() (ProductRepo, error) {
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
	Rating             Rating   `json:"nrating"`
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

func (r Review) toRepo() ReviewRepo {
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
		AvatarSmall:        links.Small,
		AvatarSmall2x:      links.Small2X,
		AvatarMedium:       links.Medium,
		AvatarMedium2x:     links.Medium2X,
		AvatarLarge:        links.Large,
		AvatarLarge2x:      links.Large2X,
		AvatarSDKImg32:     links.SdkImg32,
		AvatarSDKImg64:     links.SdkImg64,
		AvatarSDKImg184:    links.SdkImg184,
		AvatarMenuSmall:    links.MenuSmall,
		AvatarMenuSmall2:   links.MenuSmall2,
		AvatarMenuBig:      links.MenuBig,
		AvatarMenuBig2:     links.MenuBig2,
		CountersGames:      r.Reviewer.Counters.Games,
		CountersReviews:    r.Reviewer.Counters.Reviews,
		Labels:             pq.StringArray(r.Labels),
		Downvotes:          r.Votes.Downvotes,
		Upvotes:            r.Votes.Upvotes,
		ReviewDate:         r.Date,
		CreationDate:       r.CreationDate,
		InternalUpdateDate: r.InternalUpdateDate,
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
			} else {
				// It can also be empty array []any
				_, ok = l.(any)
				if !ok {
					return links, fmt.Errorf("WARN: Links are non of "+
						"expected types: %v (%T)", l, l)
				}
			}
		}
	}
	return links, nil
}

type ReviewRepo struct {
	ID                 string         `json:"id" db:"id"`
	ProductID          int            `json:"productId" db:"product_id"`
	RatingValue        int            `json:"rating" db:"rating_value"`
	Title              string         `json:"title" db:"title"`
	Description        string         `json:"description" db:"description"`
	Language           string         `json:"language" db:"language"`
	ReviewerID         string         `json:"reviewerId" db:"reviewer_id"`
	ReviewerUsername   string         `json:"reviewerUsername" db:"reviewer_username"`
	AvatarGogImageID   string         `json:"avatarGogImageId" db:"avatar_gog_image_id"`
	AvatarSmall        string         `json:"avatarSmall" db:"avatar_small"`
	AvatarSmall2x      string         `json:"avatarSmall2x" db:"avatar_small_2x"`
	AvatarMedium       string         `json:"avatarMedium" db:"avatar_medium"`
	AvatarMedium2x     string         `json:"avatarMedium2x" db:"avatar_medium_2x"`
	AvatarLarge        string         `json:"avatarLarge" db:"avatar_large"`
	AvatarLarge2x      string         `json:"avatarLarge2x" db:"avatar_large_2x"`
	AvatarSDKImg32     string         `json:"avatarSdkImg32" db:"avatar_sdk_img_32"`
	AvatarSDKImg64     string         `json:"avatarSdkImg64" db:"avatar_sdk_img_64"`
	AvatarSDKImg184    string         `json:"avatarSdkImg184" db:"avatar_sdk_img_184"`
	AvatarMenuSmall    string         `json:"avatarMenuSmall" db:"avatar_menu_small"`
	AvatarMenuSmall2   string         `json:"avatarMenuSmall2" db:"avatar_menu_small_2"`
	AvatarMenuBig      string         `json:"avatarMenuBig" db:"avatar_menu_big"`
	AvatarMenuBig2     string         `json:"avatarMenuBig2" db:"avatar_menu_big_2"`
	CountersGames      int            `json:"countersGames" db:"counters_games"`
	CountersReviews    int            `json:"countersReviews" db:"counters_reviews"`
	Labels             pq.StringArray `json:"labels" db:"labels"`
	Downvotes          int            `json:"downvotes" db:"downvotes"`
	Upvotes            int            `json:"upvotes" db:"upvotes"`
	ReviewDate         string         `json:"date" db:"review_date"`
	CreationDate       string         `json:"creationDate" db:"creation_date"`
	InternalUpdateDate string         `json:"internalUpdateDate" db:"internal_update_date"`
}

type UIGame struct {
	ID           int    `db:"id"`
	Title        string `db:"title"`
	ReviewsCount int    `db:"reviews_count"`
}

package main

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/lib/pq"
)

type CatalogResp struct {
	Pages        int       `json:"pages"`
	ProductCount int       `json:"productCount"`
	Products     []Product `json:"products"`
}

type Product struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Features []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"features"`
	Screenshots           []string `json:"screenshots"`
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
	ProductState string `json:"productState"`
	Genres       []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"genres"`
	Tags []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"tags"`
	ReviewsRating int `json:"reviewsRating"`
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
func (p Product) toRepo() (ProductRepo, error) {
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

	return ProductRepo{
		ID:                       p.ID,
		Slug:                     p.Slug,
		Features:                 mapFeatures(p.Features),
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
		Genres:                   mapGenres(p.Genres),
		Tags:                     mapTags(p.Tags),
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

// mapFeatures maps the original Features to strings
func mapFeatures(originalFeatures []struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}) []string {
	features := make([]string, len(originalFeatures))
	for i, feature := range originalFeatures {
		// If name and slug are almost the same, consider name to be equal to slug
		if feature.Name == feature.Slug {
			features[i] = feature.Slug
		} else {
			features[i] = fmt.Sprintf("%s (%s)", feature.Name, feature.Slug)
		}
	}
	return features
}

// mapGenres maps the original Genres to strings
func mapGenres(originalGenres []struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}) []string {
	genres := make([]string, len(originalGenres))
	for i, genre := range originalGenres {
		genres[i] = genre.Slug
	}
	return genres
}

// mapTags maps the original Tags to strings
func mapTags(originalTags []struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}) []string {
	tags := make([]string, len(originalTags))
	for i, tag := range originalTags {
		tags[i] = tag.Slug
	}
	return tags
}

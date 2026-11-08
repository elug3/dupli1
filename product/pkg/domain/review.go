package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Review limits (docs/product-reviews-plan.md).
const (
	ReviewMinRating            = 1
	ReviewMaxRating            = 5
	ReviewBodyMinChars         = 10
	ReviewBodyMaxChars         = 1000
	ReviewMaxPhotos            = 10
	ReviewHideReasonMaxChars   = 200
	ReviewFitMinHeightCm       = 100
	ReviewFitMaxHeightCm       = 230
	ReviewFitUsualSizeMaxChars = 10
	// ReviewAnonymousAuthor is shown when the buyer has no display name.
	ReviewAnonymousAuthor = "구매자"
)

// Fit results a clothing review may carry.
const (
	FitSmall      = "small"
	FitTrueToSize = "true_to_size"
	FitLarge      = "large"
)

// ErrInvalidReview is a review the shopper must correct (rating, text, fit or
// photo count out of range).
var ErrInvalidReview = errors.New("invalid review")

// ReviewFit is how a clothing item fits. Bags carry none.
type ReviewFit struct {
	Result    string `json:"result"`
	HeightCm  int    `json:"heightCm,omitempty"`
	UsualSize string `json:"usualSize,omitempty"`
}

// ReviewPhoto is one photo on a review, re-encoded to JPEG on upload.
type ReviewPhoto struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumbUrl"`
	// Object keys are internal: they are what deletion removes.
	ObjectKey string `json:"-"`
	ThumbKey  string `json:"-"`
	// ReviewID is empty until the photo is attached to a review.
	ReviewID   string    `json:"-"`
	CustomerID string    `json:"-"`
	CreatedAt  time.Time `json:"-"`
}

// Review is a buyer's rating of a parent product, written from one fulfilled
// order line: (OrderID, SkuID).
type Review struct {
	ID           string        `json:"id"`
	ProductID    string        `json:"productId"`
	SkuID        string        `json:"skuId"`
	VariantLabel string        `json:"variantLabel,omitempty"`
	Rating       int           `json:"rating"`
	Body         string        `json:"body"`
	Fit          *ReviewFit    `json:"fit,omitempty"`
	Photos       []ReviewPhoto `json:"photos"`
	// Author is the masked display name (김**), fixed at write time.
	Author           string     `json:"author"`
	VerifiedPurchase bool       `json:"verifiedPurchase"`
	CreatedAt        time.Time  `json:"createdAt"`
	EditedAt         *time.Time `json:"editedAt,omitempty"`

	// Author and staff fields. Stripped before a public response.
	OrderID      string     `json:"orderId,omitempty"`
	Hidden       bool       `json:"hidden,omitempty"`
	HiddenReason string     `json:"hiddenReason,omitempty"`
	CustomerID   string     `json:"customerId,omitempty"`
	HiddenBy     string     `json:"hiddenBy,omitempty"`
	HiddenAt     *time.Time `json:"hiddenAt,omitempty"`
}

// Public is the review as any visitor sees it.
func (r Review) Public() Review {
	out := r.ForAuthor()
	out.OrderID = ""
	out.Hidden = false
	out.HiddenReason = ""
	return out
}

// ForAuthor is the review as its author sees it: no staff fields.
func (r Review) ForAuthor() Review {
	out := r
	out.CustomerID = ""
	out.HiddenBy = ""
	out.HiddenAt = nil
	if out.Photos == nil {
		out.Photos = []ReviewPhoto{}
	}
	return out
}

// ReviewSummary is the rating summary of a parent product's visible reviews.
type ReviewSummary struct {
	Count      int64            `json:"count"`
	Average    *float64         `json:"average"`
	ByRating   map[string]int64 `json:"byRating"`
	PhotoCount int64            `json:"photoCount"`
	Fit        *ReviewFitCounts `json:"fit,omitempty"`
}

// ReviewFitCounts tallies the fit answers of a clothing product's reviews.
type ReviewFitCounts struct {
	Small      int64 `json:"small"`
	TrueToSize int64 `json:"trueToSize"`
	Large      int64 `json:"large"`
}

// ReviewCounters is what products stores per parent: the summary's inputs.
type ReviewCounters struct {
	Count        int64
	RatingSum    int64
	RatingCounts [5]int64 // index 0 = 1 star
	PhotoCount   int64
	FitCounts    [3]int64 // small, true_to_size, large
}

// Summary turns the stored counters into the public summary. withFit is set
// for clothing.
func (c ReviewCounters) Summary(withFit bool) ReviewSummary {
	s := ReviewSummary{
		Count:      c.Count,
		Average:    RatingAverage(c.Count, c.RatingSum),
		ByRating:   make(map[string]int64, 5),
		PhotoCount: c.PhotoCount,
	}
	for i := 0; i < 5; i++ {
		s.ByRating[string(rune('1'+i))] = c.RatingCounts[i]
	}
	if withFit {
		s.Fit = &ReviewFitCounts{Small: c.FitCounts[0], TrueToSize: c.FitCounts[1], Large: c.FitCounts[2]}
	}
	return s
}

// RatingAverage is the mean rating rounded to one decimal, or nil with no
// reviews.
func RatingAverage(count, sum int64) *float64 {
	if count <= 0 {
		return nil
	}
	avg := float64(sum) / float64(count)
	avg = float64(int64(avg*10+0.5)) / 10
	return &avg
}

// Review list sorts.
const (
	ReviewSortLatest     = "latest"
	ReviewSortRatingDesc = "rating_desc"
	ReviewSortRatingAsc  = "rating_asc"
)

// ReviewQuery filters and pages a product's visible reviews.
type ReviewQuery struct {
	Sort      string
	PhotoOnly bool
	Rating    int // 0 = any
	Limit     int
	Cursor    string
}

// ModerationQuery filters the staff review list.
type ModerationQuery struct {
	Hidden    bool
	ProductID string
	Rating    int
	Limit     int
	Cursor    string
}

// Review page sizes.
const (
	ReviewDefaultLimit       = 5
	ReviewMaxLimit           = 20
	ReviewModerationMaxLimit = 200
	ReviewPhotoDefaultLimit  = 20
	ReviewPhotoMaxLimit      = 60
)

// ClampLimit returns limit within [1, max], or def when unset.
func ClampLimit(limit, def, max int) int {
	if limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}

// NormalizeReviewSort returns a known sort, latest by default.
func NormalizeReviewSort(sort string) (string, bool) {
	switch strings.TrimSpace(sort) {
	case "", ReviewSortLatest:
		return ReviewSortLatest, true
	case ReviewSortRatingDesc:
		return ReviewSortRatingDesc, true
	case ReviewSortRatingAsc:
		return ReviewSortRatingAsc, true
	}
	return "", false
}

// ReviewInput is what a shopper writes. Pointers distinguish "not sent" on
// edit.
type ReviewInput struct {
	Rating   *int       `json:"rating"`
	Body     *string    `json:"body"`
	Fit      *ReviewFit `json:"fit"`
	PhotoIDs *[]string  `json:"photoIds"`
}

// ValidateReviewContent checks a complete review. clothing says whether the
// parent's category asks for fit.
func ValidateReviewContent(rating int, body string, fit *ReviewFit, photoCount int, clothing bool) error {
	if rating < ReviewMinRating || rating > ReviewMaxRating {
		return invalidReview("rating must be 1–5")
	}
	n := utf8.RuneCountInString(body)
	if n < ReviewBodyMinChars || n > ReviewBodyMaxChars {
		return invalidReview("body must be 10–1000 characters")
	}
	if photoCount > ReviewMaxPhotos {
		return invalidReview("at most 10 photos")
	}
	if fit != nil {
		if !clothing {
			return invalidReview("fit is for clothing only")
		}
		switch fit.Result {
		case FitSmall, FitTrueToSize, FitLarge:
		default:
			return invalidReview("fit.result must be small, true_to_size or large")
		}
		if fit.HeightCm != 0 && (fit.HeightCm < ReviewFitMinHeightCm || fit.HeightCm > ReviewFitMaxHeightCm) {
			return invalidReview("fit.heightCm must be 100–230")
		}
		if utf8.RuneCountInString(fit.UsualSize) > ReviewFitUsualSizeMaxChars {
			return invalidReview("fit.usualSize is at most 10 characters")
		}
	}
	return nil
}

// NormalizeReviewFit trims a fit, and drops one that carries nothing.
func NormalizeReviewFit(fit *ReviewFit) *ReviewFit {
	if fit == nil {
		return nil
	}
	out := *fit
	out.Result = strings.TrimSpace(out.Result)
	out.UsualSize = strings.TrimSpace(out.UsualSize)
	if out.Result == "" && out.HeightCm == 0 && out.UsualSize == "" {
		return nil
	}
	return &out
}

// FitIndex is the counter slot of a fit result, or -1.
func FitIndex(result string) int {
	switch result {
	case FitSmall:
		return 0
	case FitTrueToSize:
		return 1
	case FitLarge:
		return 2
	}
	return -1
}

// MaskAuthor keeps the first character of a display name: 김민지 → 김**.
// The full name is never stored. Empty → 구매자.
func MaskAuthor(displayName string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return ReviewAnonymousAuthor
	}
	first, _ := utf8.DecodeRuneInString(name)
	return string(first) + "**"
}

// IsClothing reports whether a parent's category asks reviews for fit.
func IsClothing(category string) bool {
	return strings.EqualFold(strings.TrimSpace(category), "clothing")
}

func invalidReview(msg string) error {
	return &reviewError{msg: msg}
}

type reviewError struct{ msg string }

func (e *reviewError) Error() string { return "invalid review: " + e.msg }
func (e *reviewError) Unwrap() error { return ErrInvalidReview }

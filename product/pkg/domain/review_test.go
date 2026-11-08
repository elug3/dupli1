package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestMaskAuthor(t *testing.T) {
	for in, want := range map[string]string{
		"김민지":     "김**",
		" Alice ": "A**",
		"":        ReviewAnonymousAuthor,
		"   ":     ReviewAnonymousAuthor,
	} {
		if got := MaskAuthor(in); got != want {
			t.Errorf("MaskAuthor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateReviewContent(t *testing.T) {
	body := "가죽 결이 곱고 좋아요."
	ok := []struct {
		rating   int
		body     string
		fit      *ReviewFit
		photos   int
		clothing bool
	}{
		{5, body, nil, 10, false},
		{1, strings.Repeat("가", 1000), nil, 0, false},
		{3, body, &ReviewFit{Result: FitTrueToSize, HeightCm: 175, UsualSize: "M"}, 0, true},
	}
	for _, c := range ok {
		if err := ValidateReviewContent(c.rating, c.body, c.fit, c.photos, c.clothing); err != nil {
			t.Errorf("valid review refused: %v", err)
		}
	}
	bad := []struct {
		name     string
		rating   int
		body     string
		fit      *ReviewFit
		photos   int
		clothing bool
	}{
		{"rating 6", 6, body, nil, 0, false},
		{"9 characters", 5, "가나다라마바사아자", nil, 0, false},
		{"1001 characters", 5, strings.Repeat("가", 1001), nil, 0, false},
		{"11 photos", 5, body, nil, 11, false},
		{"fit on a bag", 5, body, &ReviewFit{Result: FitSmall}, 0, false},
		{"unknown fit", 5, body, &ReviewFit{Result: "huge"}, 0, true},
		{"height 90", 5, body, &ReviewFit{Result: FitSmall, HeightCm: 90}, 0, true},
	}
	for _, c := range bad {
		if err := ValidateReviewContent(c.rating, c.body, c.fit, c.photos, c.clothing); !errors.Is(err, ErrInvalidReview) {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
}

func TestRatingAverageRoundsToOneDecimal(t *testing.T) {
	if RatingAverage(0, 0) != nil {
		t.Fatal("average with no reviews must be nil")
	}
	if got := *RatingAverage(3, 14); got != 4.7 {
		t.Fatalf("14/3 = %v, want 4.7", got)
	}
	s := ReviewCounters{Count: 2, RatingSum: 9, RatingCounts: [5]int64{0, 0, 0, 1, 1}}.Summary(false)
	if s.ByRating["4"] != 1 || s.ByRating["5"] != 1 || s.ByRating["1"] != 0 || s.Fit != nil {
		t.Fatalf("summary: %+v", s)
	}
}

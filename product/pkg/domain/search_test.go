package domain

import (
	"testing"
	"time"
)

func TestNormalizeSearchSort(t *testing.T) {
	cases := map[string]string{
		"":           SortNewest,
		"newest":     SortNewest,
		"NEW":        SortNewest,
		"popular":    SortViews,
		"views":      SortViews,
		"sold":       SortSold,
		"wishlist":   SortWishlist,
		"price":      SortPrice,
		"name":       SortName,
		"nope":       "",
	}
	for in, want := range cases {
		if got := NormalizeSearchSort(in); got != want {
			t.Errorf("NormalizeSearchSort(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestNormalizeSearchOrder(t *testing.T) {
	if got := NormalizeSearchOrder("", SortName); got != OrderAsc {
		t.Fatalf("name default order: want asc, got %s", got)
	}
	if got := NormalizeSearchOrder("", SortViews); got != OrderDesc {
		t.Fatalf("views default order: want desc, got %s", got)
	}
	if got := NormalizeSearchOrder("ASC", SortViews); got != OrderAsc {
		t.Fatalf("want asc, got %s", got)
	}
	if got := NormalizeSearchOrder("bogus", SortViews); got != "" {
		t.Fatalf("want empty for invalid, got %s", got)
	}
}

func TestNormalizeSearchPeriod(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"week":       PeriodWeek,
		"7d":         PeriodWeek,
		"past_week":  PeriodWeek,
		"day":        PeriodDay,
		"month":      PeriodMonth,
		"nope":       "",
	}
	for in, want := range cases {
		if got := NormalizeSearchPeriod(in); got != want {
			t.Errorf("NormalizeSearchPeriod(%q)=%q, want %q", in, got, want)
		}
	}
	cutoff, ok := SearchPeriodCutoff(PeriodWeek, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected cutoff")
	}
	want := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	if !cutoff.Equal(want) {
		t.Fatalf("week cutoff=%s, want %s", cutoff, want)
	}
}

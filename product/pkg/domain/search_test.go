package domain

import "testing"

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

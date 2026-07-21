package domain

import (
	"strings"
	"time"
)

// Allowed product list sort keys (query ?sort=).
const (
	SortNewest   = "newest"
	SortViews    = "views"
	SortSold     = "sold"
	SortWishlist = "wishlist"
	SortPrice    = "price"
	SortName     = "name"
)

// Allowed order directions (query ?order=).
const (
	OrderAsc  = "asc"
	OrderDesc = "desc"
)

// Allowed created-at window filters (query ?period=).
const (
	PeriodDay   = "day"
	PeriodWeek  = "week"
	PeriodMonth = "month"
)

// NormalizeSearchSort maps aliases and defaults to a canonical sort key.
// Empty or unknown values become SortNewest (callers that want 400 should
// validate with ValidSearchSort first).
func NormalizeSearchSort(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", SortNewest, "created", "created_at", "new":
		return SortNewest
	case SortViews, "popular", "view", "view_count", "viewcount":
		return SortViews
	case SortSold, "sales", "sold_count", "soldcount":
		return SortSold
	case SortWishlist, "wish", "wishlist_count", "wishlistcount":
		return SortWishlist
	case SortPrice, "price_from", "pricefrom":
		return SortPrice
	case SortName, "title":
		return SortName
	default:
		return ""
	}
}

// ValidSearchSort reports whether raw is empty (default newest) or a known sort.
func ValidSearchSort(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	return NormalizeSearchSort(raw) != ""
}

// NormalizeSearchOrder returns asc or desc. Empty uses the default for sortKey.
func NormalizeSearchOrder(raw, sortKey string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case OrderAsc, "ascending":
		return OrderAsc
	case OrderDesc, "descending":
		return OrderDesc
	case "":
		if sortKey == SortName {
			return OrderAsc
		}
		return OrderDesc
	default:
		return ""
	}
}

// ValidSearchOrder reports whether raw is empty (default) or a known direction.
func ValidSearchOrder(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	return NormalizeSearchOrder(raw, SortNewest) != ""
}

// NormalizeSearchPeriod maps aliases to a canonical period key.
// Empty means no window filter; unknown returns "".
func NormalizeSearchPeriod(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case PeriodDay, "1d", "today", "past_day", "last_day":
		return PeriodDay
	case PeriodWeek, "7d", "past_week", "last_week", "pastweek":
		return PeriodWeek
	case PeriodMonth, "30d", "past_month", "last_month", "pastmonth":
		return PeriodMonth
	default:
		return ""
	}
}

// ValidSearchPeriod reports whether raw is empty (no filter) or a known period.
func ValidSearchPeriod(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	return NormalizeSearchPeriod(raw) != ""
}

// SearchPeriodDuration returns how far back a period reaches from "now".
func SearchPeriodDuration(period string) time.Duration {
	switch NormalizeSearchPeriod(period) {
	case PeriodDay:
		return 24 * time.Hour
	case PeriodWeek:
		return 7 * 24 * time.Hour
	case PeriodMonth:
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}

// SearchPeriodCutoff returns the inclusive lower bound for created_at, or ok=false
// when period is empty/unknown.
func SearchPeriodCutoff(period string, now time.Time) (time.Time, bool) {
	d := SearchPeriodDuration(period)
	if d == 0 {
		return time.Time{}, false
	}
	return now.UTC().Add(-d), true
}

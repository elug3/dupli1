package domain

import "strings"

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

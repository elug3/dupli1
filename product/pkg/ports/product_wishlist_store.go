package ports

// ProductWishlistStore records unique wishlists per owner and maintains denormalized counters.
type ProductWishlistStore interface {
	// AddWishlist inserts (ownerKey, productID) if new. When inserted, products.wishlist_count
	// is incremented. Returns whether this call added a new row and the current count.
	AddWishlist(ownerKey, productID string) (added bool, wishlistCount int64, err error)
	// RemoveWishlist deletes (ownerKey, productID) if present and decrements wishlist_count.
	RemoveWishlist(ownerKey, productID string) (removed bool, wishlistCount int64, err error)
	// ListWishlistProductIDs returns parent product ids wishlisted by ownerKey (newest first).
	ListWishlistProductIDs(ownerKey string) ([]string, error)
}

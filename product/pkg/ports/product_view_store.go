package ports

// ProductViewStore records unique guest PDP views and maintains denormalized counters.
type ProductViewStore interface {
	// RecordUniqueView inserts (guestID, productID) if new. When a row is inserted,
	// products.view_count is incremented. Returns whether this call counted as a new
	// view and the current products.view_count after the write (for fresh PDP JSON).
	RecordUniqueView(guestID, productID string) (inserted bool, viewCount int64, err error)
}

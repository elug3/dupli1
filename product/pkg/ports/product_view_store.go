package ports

// ProductViewStore records unique guest PDP views and maintains denormalized counters.
type ProductViewStore interface {
	// RecordUniqueView inserts (guestID, productID) if new. When a row is inserted,
	// products.view_count is incremented. Returns whether this call counted as a new view.
	RecordUniqueView(guestID, productID string) (inserted bool, err error)
}

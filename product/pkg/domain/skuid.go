package domain

import (
	"github.com/oklog/ulid/v2"
)

// NewSkuID returns a new canonical, sortable, cross-service sku identifier.
func NewSkuID() string {
	return ulid.Make().String()
}

// NewProductID returns a new canonical, sortable parent product identifier (ULID).
// Human brand/style identity lives on brandCode + styleCode, not on this id.
func NewProductID() string {
	return ulid.Make().String()
}

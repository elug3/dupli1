package domain

import (
	"github.com/oklog/ulid/v2"
)

// NewSkuID returns a new canonical, sortable, cross-service sku identifier.
func NewSkuID() string {
	return ulid.Make().String()
}

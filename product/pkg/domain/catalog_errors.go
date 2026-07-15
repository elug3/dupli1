package domain

import "errors"

// ErrMasterInUse is returned when deleting a catalog master that is still
// referenced by styles, products, or product_variants.
var ErrMasterInUse = errors.New("master data is in use")

// ErrMasterExists is returned when creating a master whose code already exists.
var ErrMasterExists = errors.New("master data already exists")

// ErrMasterNotFound is returned when a master code does not exist.
var ErrMasterNotFound = errors.New("master data not found")

// ErrMissingSKUCodes is returned when product/variant create omits required codes.
var ErrMissingSKUCodes = errors.New("required sku master codes missing")

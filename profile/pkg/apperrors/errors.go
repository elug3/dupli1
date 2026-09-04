// Package apperrors holds service-level sentinel errors for the profile
// service that don't belong in domain (business-rule invariants that span
// beyond a single field validation) or ports (repository-shape errors).
package apperrors

import "errors"

var (
	// ErrAddressLimitReached is returned when a user tries to save more than
	// domain.MaxAddressesPerUser addresses.
	ErrAddressLimitReached = errors.New("address limit reached")
)

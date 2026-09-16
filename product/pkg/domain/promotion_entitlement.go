package domain

import "time"

// CustomerPromotion is one account's right to use a single-user promotional
// code. A global code needs no entitlement; a single-user one is unusable
// without it.
//
// Note what this deliberately does **not** track: whether the code has been
// used. That lives in the redemption ledger, which already decides it inside a
// transaction and can tell a retry from a second use. Duplicating it here
// would create a second writer for the same fact, and the two would drift the
// first time a cancel released a use. The plan sketched a fuller status set;
// this is the narrower version that keeps one source of truth.
type CustomerPromotion struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Code       string `json:"code"`

	// Source records how the entitlement was granted, for support and
	// reporting: "system" for the registration issuer, "backfill" for the
	// one-off run over existing accounts, "issue" for a manager.
	Source string `json:"source"`
	// TriggerKey makes issuing idempotent. A redelivered user.registered
	// event carries the same key and mints nothing.
	TriggerKey string `json:"trigger_key,omitempty"`
	IssuedBy   string `json:"issued_by,omitempty"`

	// ExpiresAt is per entitlement, not per campaign, so an account issued
	// today gets the same window as one issued at launch.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// RevokedAt marks an entitlement withdrawn — issued by mistake, say.
	// Revoking never rewrites an order that already used it.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Usable reports whether this entitlement still grants access to its code.
// It says nothing about whether the customer has already spent it — ask the
// ledger for that.
func (e CustomerPromotion) Usable(now time.Time) bool {
	if e.RevokedAt != nil {
		return false
	}
	if e.ExpiresAt != nil && now.After(*e.ExpiresAt) {
		return false
	}
	return true
}

// EntitlementExpiry is when an entitlement issued now should lapse, given the
// definition's window. A definition with no window issues entitlements that do
// not expire on their own.
func (p Promotion) EntitlementExpiry(now time.Time) *time.Time {
	if p.EntitlementTTLDays <= 0 {
		return nil
	}
	at := now.AddDate(0, 0, p.EntitlementTTLDays).UTC()
	return &at
}

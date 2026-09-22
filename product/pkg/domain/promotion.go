package domain

import (
	"strings"
	"time"
)

// Scope is the audience of a promotional code.
type Scope string

const (
	// ScopeGlobal is a shared campaign code anyone may use once. Every code
	// written before Phase 2 is this.
	ScopeGlobal Scope = "global"
	// ScopeSingleUser is bound to one account through an entitlement. Phase 3.
	ScopeSingleUser Scope = "single_user"
)

// KST is Asia/Seoul. Fixed rather than loaded from the tz database: Korea has
// had no DST since 1988, and a fixed zone works in a scratch container with no
// tzdata installed.
var KST = time.FixedZone("KST", 9*60*60)

type Promotion struct {
	Code        string `json:"code"`
	Scope       Scope  `json:"scope"`
	Description string `json:"description"`
	Active      bool   `json:"active"`

	// Conditions decide eligibility; Benefit decides the money.
	Conditions Conditions `json:"conditions"`
	Benefit    Benefit    `json:"benefit"`

	// ExpiresAt is stored UTC and compared in UTC. Managers author a date,
	// which means the end of that day in KST — see EndOfDayKST.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// MaxRedemptions caps a campaign's total paid uses. nil is uncapped.
	MaxRedemptions *int `json:"max_redemptions,omitempty"`
	// MaxPerCustomer is 1 for both scopes today; the field exists so raising
	// it later is data rather than a migration.
	MaxPerCustomer int `json:"max_per_customer"`
	// RedemptionCount is denormalised from the ledger for cheap list views and
	// the campaign cap check.
	RedemptionCount int `json:"redemption_count"`

	// EntitlementTTLDays is how long an issued entitlement lasts, in days.
	// Only meaningful for single_user codes. Per entitlement rather than per
	// campaign, so an account issued late in a campaign gets the same window
	// as one issued at launch. 0 means the entitlement does not expire on its
	// own (the definition's ExpiresAt, if any, still applies).
	EntitlementTTLDays int `json:"entitlement_ttl_days,omitempty"`

	// Terms is customer-facing copy stating what the code requires, shown at
	// redeem, in the wallet and in the checkout summary. A campaign with a
	// minimum spend or an expiry should say so here.
	Terms string `json:"terms,omitempty"`

	UpdatedAt time.Time `json:"updated_at,omitempty"`

	// Discount and Expires are the pre-Phase-2 columns. Nothing writes them
	// now — Benefit and ExpiresAt do — but they are still read so a row
	// written by an older process still prices correctly.
	// See docs/product-promo-referral-code-plan.md.
	Discount float64 `json:"discount"`
	Expires  string  `json:"expires"`
}

// WelcomeCode is the sign-up campaign: the single-user code every new
// customer is issued when auth publishes user.registered.
//
// It is a constant rather than configuration because nothing about it is
// per-environment. Both stores seed the definition, so it always exists, and
// whether customers can spend it is the definition's own `active` flag — a
// manager's switch, not a deploy's. A third switch in the environment could
// only ever disagree with those two, and did: unset, the issuer never
// subscribed and no registration got a code, with nothing on any screen to
// say so.
const WelcomeCode = "WELCOME50"

// NormalizedCode is the storage and lookup form of a code.
func NormalizedCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// EffectiveBenefit returns the stored benefit, falling back to the legacy
// percentage column for rows that predate Phase 2.
func (p Promotion) EffectiveBenefit() Benefit {
	if !p.Benefit.IsZero() {
		return p.Benefit
	}
	return Benefit{
		Target:           BenefitTargetGoods,
		DiscountType:     DiscountTypePercent,
		DiscountFraction: p.Discount,
		ApplyTo:          ApplyToEntireSubtotal,
	}
}

// EffectiveScope defaults an unset scope to global, which is what every
// pre-Phase-2 row is.
func (p Promotion) EffectiveScope() Scope {
	if p.Scope == "" {
		return ScopeGlobal
	}
	return p.Scope
}

// IsExpired reports whether the code's window has closed.
//
// A code with no ExpiresAt never expires. The legacy Expires column is free
// text that was never comparable — it is displayed, not enforced — so it is
// deliberately not consulted here: a definition must be given a real
// ExpiresAt to be enforced.
func (p Promotion) IsExpired(now time.Time) bool {
	if p.ExpiresAt == nil {
		return false
	}
	return now.After(*p.ExpiresAt)
}

// IsExhausted reports whether the campaign-wide cap is spent.
func (p Promotion) IsExhausted() bool {
	if p.MaxRedemptions == nil {
		return false
	}
	return p.RedemptionCount >= *p.MaxRedemptions
}

// EffectiveMaxPerCustomer defaults to one use each.
func (p Promotion) EffectiveMaxPerCustomer() int {
	if p.MaxPerCustomer <= 0 {
		return 1
	}
	return p.MaxPerCustomer
}

// EndOfDayKST turns a manager's date ("2026-08-31") into the instant that day
// ends in Korea, as UTC. A manager picking a date means the code works all the
// way through it, so the boundary is 23:59:59.999999999 KST and not midnight.
func EndOfDayKST(date string) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), KST)
	if err != nil {
		return time.Time{}, err
	}
	return day.Add(24*time.Hour - time.Nanosecond).UTC(), nil
}

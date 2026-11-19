package domain

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// PromotionKind separates ordinary promotional codes from referral codes.
//
// A referral code is a secret code handed to an acquaintance of dupli1 (the
// referrer). Shoppers type it in the same promotional code box as any other
// code and the storefront never names it as a referral. It discounts the goods
// like a global code, but a shopper may use it on any number of orders, and
// every fulfilled order earns the referrer a cash commission that staff pay by
// hand each month (order's referral report). See docs/product-referral-codes.md.
type PromotionKind string

const (
	// PromotionKindStandard is every promotional code that is not a referral.
	PromotionKindStandard PromotionKind = ""
	// PromotionKindReferral is a secret referral code with a named referrer.
	PromotionKindReferral PromotionKind = "referral"
)

// Valid reports whether k is a kind product understands.
func (k PromotionKind) Valid() bool {
	switch k {
	case PromotionKindStandard, PromotionKindReferral:
		return true
	}
	return false
}

const (
	// DefaultReferralCommissionBps is the commission a referral code pays when
	// a manager does not set one: 5% (500 basis points).
	DefaultReferralCommissionBps = 500
	// MaxReferralCommissionBps is 100%.
	MaxReferralCommissionBps = 10000
	// MaxReferrerNameLen bounds the referrer's name, in characters.
	MaxReferrerNameLen = 80
)

// IsReferral reports whether this definition is a referral code.
func (p Promotion) IsReferral() bool { return p.Kind == PromotionKindReferral }

// CustomerLimit is how many uses one customer may hold at once, as the ledger
// enforces it. A referral code has no per-customer limit: the same shopper may
// use it on every order. EffectiveMaxPerCustomer is still what is stored.
func (p Promotion) CustomerLimit() int {
	if p.IsReferral() {
		return math.MaxInt32
	}
	return p.EffectiveMaxPerCustomer()
}

// Referral is what order snapshots onto an order placed with a referral code:
// who referred it and the commission rate at that moment, so a later edit to
// the code never changes what a past order earns.
type Referral struct {
	ReferrerName  string `json:"referrer_name"`
	CommissionBps int    `json:"commission_bps"`
}

// ReferralSnapshot returns the referral terms for an order, nil for an
// ordinary code.
func (p Promotion) ReferralSnapshot() *Referral {
	if !p.IsReferral() {
		return nil
	}
	return &Referral{ReferrerName: p.ReferrerName, CommissionBps: p.CommissionBps}
}

// PublicView is the definition as anyone who can type a code may see it. A
// referral code is a secret feature, so its kind, referrer and commission are
// dropped: a referral code looks like any other code to a shopper.
func (p Promotion) PublicView() Promotion {
	p.Kind = PromotionKindStandard
	p.ReferrerName = ""
	p.CommissionBps = 0
	return p
}

// ValidateReferral checks the referral fields of a definition. An ordinary
// code must carry none of them, so a referrer name cannot ride along on a
// code that pays no commission.
func ValidateReferral(p Promotion) error {
	if !p.Kind.Valid() {
		return fmt.Errorf("kind %q is not one of \"\", referral", p.Kind)
	}
	if !p.IsReferral() {
		if strings.TrimSpace(p.ReferrerName) != "" || p.CommissionBps != 0 {
			return fmt.Errorf("referrer_name and commission_bps are only for referral codes (kind referral)")
		}
		return nil
	}
	name := strings.TrimSpace(p.ReferrerName)
	if name == "" {
		return fmt.Errorf("a referral code needs a referrer_name")
	}
	if utf8.RuneCountInString(name) > MaxReferrerNameLen {
		return fmt.Errorf("referrer_name must be at most %d characters", MaxReferrerNameLen)
	}
	if p.CommissionBps < 0 || p.CommissionBps > MaxReferralCommissionBps {
		return fmt.Errorf("commission_bps must be between 0 and %d, got %d", MaxReferralCommissionBps, p.CommissionBps)
	}
	if p.EffectiveScope() != ScopeGlobal {
		return fmt.Errorf("a referral code must be global: it is typed in the code box, not issued to an account")
	}
	if p.AutoIssue != AutoIssueNone {
		return fmt.Errorf("a referral code cannot be auto-issued")
	}
	if p.IsTier() {
		return fmt.Errorf("a referral code cannot be an automatic tier")
	}
	return nil
}

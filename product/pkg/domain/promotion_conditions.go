package domain

import (
	"fmt"
	"strings"
)

// Conditions decide whether a promotional code may be applied to a given
// checkout. They are stored as a versioned JSONB document on the definition so
// new rules are data, not new columns — see
// docs/product-promo-referral-code-plan.md.
//
// An empty document is always eligible, which is what every code carried
// before Phase 2.

// ConditionsVersion is the only document shape understood today. A document
// declaring anything else is rejected on write rather than guessed at.
const ConditionsVersion = 1

// Op is a comparison in a predicate.
type Op string

const (
	OpEq  Op = "eq"
	OpNeq Op = "neq"
	OpIn  Op = "in"
	OpNin Op = "nin"
	OpGte Op = "gte"
	OpLte Op = "lte"
	OpGt  Op = "gt"
	OpLt  Op = "lt"
)

func (o Op) valid() bool {
	switch o {
	case OpEq, OpNeq, OpIn, OpNin, OpGte, OpLte, OpGt, OpLt:
		return true
	}
	return false
}

// numeric reports whether the op orders its operands, which only numbers do.
func (o Op) numeric() bool {
	switch o {
	case OpGte, OpLte, OpGt, OpLt:
		return true
	}
	return false
}

// Attributes a predicate may address. The set is an allowlist, not a free
// expression language: a manager cannot reach a field the evaluator does not
// know how to read, and adding one is a deliberate change here.
const (
	AttrSubtotalWon    = "subtotal_won"
	AttrShippingFeeWon = "shipping_fee_won"
	AttrItemCount      = "item_count"

	AttrLineCategory     = "line.category"
	AttrLineBrandCode    = "line.brandCode"
	AttrLineUnitPriceWon = "line.unit_price_won"
	AttrLineSkuID        = "line.skuId"
	AttrLineProductID    = "line.productId"
	AttrLineOnSale       = "line.on_sale"
)

// attrKind says how an attribute's value is compared, so a predicate can be
// checked on write instead of failing strangely at checkout.
type attrKind int

const (
	kindNumber attrKind = iota
	kindString
	kindBool
)

// Every attribute here is one the evaluator can actually read: the money and
// shape of the checkout it is given, and the catalog attributes it resolves
// itself. A `customer.paid_order_count` was allowed briefly and removed on
// 2026-09-21 — only order knows that number, so a rule on it was refused on
// every cart, and a manager could pick a condition nothing could satisfy. See
// docs/product-promo-referral-code-plan.md.
var allowedAttrs = map[string]attrKind{
	AttrSubtotalWon:      kindNumber,
	AttrShippingFeeWon:   kindNumber,
	AttrItemCount:        kindNumber,
	AttrLineCategory:     kindString,
	AttrLineBrandCode:    kindString,
	AttrLineUnitPriceWon: kindNumber,
	AttrLineSkuID:        kindString,
	AttrLineProductID:    kindString,
	AttrLineOnSale:       kindBool,
}

// IsLineAttr reports whether an attribute is read per cart line rather than
// once for the whole checkout.
func IsLineAttr(attr string) bool { return strings.HasPrefix(attr, "line.") }

// Predicate is one comparison: attr op value.
type Predicate struct {
	Attr  string `json:"attr"`
	Op    Op     `json:"op"`
	Value any    `json:"value"`
}

// LineMatch says how line predicates combine across the cart.
type LineMatch string

const (
	// LineMatchAny — eligible when at least one line matches. The default.
	LineMatchAny LineMatch = "any"
	// LineMatchAll — every line must match, e.g. a whole-cart brand rule.
	LineMatchAll LineMatch = "all"
	// LineMatchEligibleOnly gates like LineMatchAny. It reads as intent at the
	// definition site — "only the matching lines matter" — while the discount
	// base is actually chosen by Benefit.ApplyTo.
	LineMatchEligibleOnly LineMatch = "eligible_only"
)

func (m LineMatch) valid() bool {
	switch m {
	case "", LineMatchAny, LineMatchAll, LineMatchEligibleOnly:
		return true
	}
	return false
}

// Conditions is the stored eligibility document.
type Conditions struct {
	Version   int         `json:"version"`
	All       []Predicate `json:"all,omitempty"`
	LineMatch LineMatch   `json:"line_match,omitempty"`
	Exclude   []Predicate `json:"exclude,omitempty"`
}

// IsEmpty reports a document with no rules — always eligible.
func (c Conditions) IsEmpty() bool { return len(c.All) == 0 && len(c.Exclude) == 0 }

// Validate checks a document on write so a manager cannot save a code that
// fails only later, at somebody's checkout.
func (c Conditions) Validate() error {
	if c.IsEmpty() && c.Version == 0 {
		return nil
	}
	if c.Version != ConditionsVersion {
		return fmt.Errorf("conditions version %d is not supported (want %d)", c.Version, ConditionsVersion)
	}
	if !c.LineMatch.valid() {
		return fmt.Errorf("line_match %q is not one of any, all, eligible_only", c.LineMatch)
	}
	for _, list := range [][]Predicate{c.All, c.Exclude} {
		for _, p := range list {
			if err := p.validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p Predicate) validate() error {
	kind, ok := allowedAttrs[p.Attr]
	if !ok {
		return fmt.Errorf("attr %q is not an allowed condition attribute", p.Attr)
	}
	if !p.Op.valid() {
		return fmt.Errorf("op %q is not a known comparison", p.Op)
	}
	if p.Value == nil {
		return fmt.Errorf("predicate on %q needs a value", p.Attr)
	}
	if p.Op.numeric() && kind != kindNumber {
		return fmt.Errorf("op %q needs a numeric attribute, but %q is not", p.Op, p.Attr)
	}
	switch p.Op {
	case OpIn, OpNin:
		if _, ok := p.Value.([]any); !ok {
			return fmt.Errorf("op %q on %q needs a list value", p.Op, p.Attr)
		}
	default:
		if _, ok := p.Value.([]any); ok {
			return fmt.Errorf("op %q on %q needs a single value, not a list", p.Op, p.Attr)
		}
	}
	if kind == kindNumber && !p.Op.isSetOp() {
		if _, ok := toNumber(p.Value); !ok {
			return fmt.Errorf("predicate on %q needs a number, got %T", p.Attr, p.Value)
		}
	}
	return nil
}

func (o Op) isSetOp() bool { return o == OpIn || o == OpNin }

// CatalogAttrs are the attributes whose values come from the catalog rather
// than from the checkout being judged.
var CatalogAttrs = []string{
	AttrLineCategory,
	AttrLineBrandCode,
	AttrLineProductID,
	AttrLineOnSale,
}

// NeedsCatalog reports whether any predicate reads a catalog attribute.
//
// The evaluator uses this to decide whether an evaluation has to look lines up
// at all: most codes gate on money alone and should not pay for a catalog read.
func (c Conditions) NeedsCatalog() bool {
	for _, list := range [][]Predicate{c.All, c.Exclude} {
		for _, p := range list {
			for _, attr := range CatalogAttrs {
				if p.Attr == attr {
					return true
				}
			}
		}
	}
	return false
}

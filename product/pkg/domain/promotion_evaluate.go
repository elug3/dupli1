package domain

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Reason is a machine-readable rejection, so customer copy lives in the
// frontends rather than in this service's error strings.
type Reason string

const (
	// ReasonInvalidCode covers unknown, inactive and soft-deleted codes alike.
	// They are deliberately indistinguishable: telling them apart would let
	// anyone enumerate live campaigns.
	ReasonInvalidCode       Reason = "invalid_code"
	ReasonExpired           Reason = "expired"
	ReasonAlreadyUsed       Reason = "already_used"
	ReasonNotEligible       Reason = "not_eligible"
	ReasonCampaignExhausted Reason = "campaign_exhausted"
	ReasonLoginRequired     Reason = "login_required"
)

// Sub-reasons refine ReasonNotEligible so a storefront can say which rule bit.
const (
	SubReasonMinSpend    = "min_spend"
	SubReasonCategory    = "category"
	SubReasonBrand       = "brand"
	SubReasonOnSale      = "on_sale_excluded"
	SubReasonNoLineMatch = "no_line_match"
	SubReasonCondition   = "condition"
)

// EvaluationLine is one cart line as the evaluator sees it. Order builds these
// from a checkout session after server-side pricing, so UnitPriceWon is the
// resolved selling price, never a client-supplied number.
type EvaluationLine struct {
	SkuID        string `json:"sku_id"`
	SKU          string `json:"sku"`
	ProductID    string `json:"product_id"`
	Category     string `json:"category"`
	BrandCode    string `json:"brand_code"`
	Quantity     int    `json:"quantity"`
	UnitPriceWon int64  `json:"unit_price_won"`
	OnSale       bool   `json:"on_sale"`
}

// ExtendedWon is the line total.
func (l EvaluationLine) ExtendedWon() int64 { return int64(l.Quantity) * l.UnitPriceWon }

// EvaluationContext is the checkout a code is being judged against.
type EvaluationContext struct {
	CustomerID     string           `json:"customer_id"`
	ShippingFeeWon int64            `json:"shipping_fee_won"`
	Lines          []EvaluationLine `json:"lines"`
	// PaidOrderCount is nil when the caller could not determine it. A
	// first-order-only rule then fails closed rather than handing out a
	// discount on an unknown history.
	PaidOrderCount *int      `json:"paid_order_count,omitempty"`
	Now            time.Time `json:"-"`
}

// SubtotalWon sums the lines. It is derived rather than passed in so a caller
// cannot claim a subtotal its lines do not add up to.
func (c EvaluationContext) SubtotalWon() int64 {
	var total int64
	for _, line := range c.Lines {
		total += line.ExtendedWon()
	}
	return total
}

func (c EvaluationContext) itemCount() int64 {
	var n int64
	for _, line := range c.Lines {
		n += int64(line.Quantity)
	}
	return n
}

// EvaluationResult is the verdict plus the money it implies.
type EvaluationResult struct {
	OK                  bool     `json:"ok"`
	DiscountWon         int64    `json:"discount_won"`
	ShippingDiscountWon int64    `json:"shipping_discount_won"`
	EligibleSkuIDs      []string `json:"eligible_sku_ids,omitempty"`
	EligibleSubtotalWon int64    `json:"eligible_subtotal_won"`
	Reason              Reason   `json:"reason,omitempty"`
	SubReason           string   `json:"sub_reason,omitempty"`
	// AppliedBenefit is the benefit exactly as it was when this evaluation
	// ran. The ledger stores it so a later edit to the definition cannot
	// rewrite what a past order was given.
	AppliedBenefit *Benefit `json:"applied_benefit,omitempty"`
}

func reject(reason Reason, sub string) EvaluationResult {
	return EvaluationResult{Reason: reason, SubReason: sub}
}

// Evaluate decides whether the promotion applies to ctx and computes the
// discount if it does.
//
// It deliberately does not know about the redemption ledger: once-per-customer
// and entitlement ownership need stored state, so the service layer checks
// those and this stays a pure function of definition + cart.
func (p Promotion) Evaluate(ctx EvaluationContext) EvaluationResult {
	if !p.Active {
		return reject(ReasonInvalidCode, "")
	}
	if p.IsExpired(ctx.Now) {
		return reject(ReasonExpired, "")
	}
	if p.IsExhausted() {
		return reject(ReasonCampaignExhausted, "")
	}
	if len(ctx.Lines) == 0 {
		return reject(ReasonNotEligible, SubReasonNoLineMatch)
	}

	eligible, res := p.eligibleLines(ctx)
	if !res.OK {
		return res
	}

	benefit := p.EffectiveBenefit()
	base := benefit.base(ctx, eligible)
	discount := benefit.discountFor(base)

	return EvaluationResult{
		OK:                  true,
		DiscountWon:         discount,
		EligibleSkuIDs:      skuIDsOf(eligible),
		EligibleSubtotalWon: base,
		AppliedBenefit:      &benefit,
	}
}

// eligibleLines applies the condition document and returns the lines the
// benefit may draw on.
func (p Promotion) eligibleLines(ctx EvaluationContext) ([]EvaluationLine, EvaluationResult) {
	cond := p.Conditions
	if cond.IsEmpty() {
		return ctx.Lines, EvaluationResult{OK: true}
	}

	// Cart- and customer-level predicates gate the whole checkout.
	for _, pred := range cond.All {
		if IsLineAttr(pred.Attr) {
			continue
		}
		ok, known := pred.matchScalar(ctx)
		if !known || !ok {
			return nil, reject(ReasonNotEligible, subReasonFor(pred))
		}
	}

	linePreds := filterLinePreds(cond.All)
	excludePreds := cond.Exclude

	var eligible []EvaluationLine
	for _, line := range ctx.Lines {
		if !lineMatchesAll(line, linePreds, ctx) {
			continue
		}
		if lineMatchesAny(line, excludePreds, ctx) {
			continue
		}
		eligible = append(eligible, line)
	}

	switch cond.LineMatch {
	case LineMatchAll:
		if len(eligible) != len(ctx.Lines) {
			return nil, reject(ReasonNotEligible, lineSubReason(linePreds, excludePreds))
		}
	default: // any, eligible_only, unset
		if len(eligible) == 0 {
			return nil, reject(ReasonNotEligible, lineSubReason(linePreds, excludePreds))
		}
	}
	return eligible, EvaluationResult{OK: true}
}

func filterLinePreds(all []Predicate) []Predicate {
	var out []Predicate
	for _, p := range all {
		if IsLineAttr(p.Attr) {
			out = append(out, p)
		}
	}
	return out
}

func lineMatchesAll(line EvaluationLine, preds []Predicate, ctx EvaluationContext) bool {
	for _, p := range preds {
		ok, known := p.matchLine(line, ctx)
		if !known || !ok {
			return false
		}
	}
	return true
}

func lineMatchesAny(line EvaluationLine, preds []Predicate, ctx EvaluationContext) bool {
	for _, p := range preds {
		if ok, known := p.matchLine(line, ctx); known && ok {
			return true
		}
	}
	return false
}

// subReasonFor maps a failed predicate to the closest customer-facing reason.
func subReasonFor(p Predicate) string {
	switch p.Attr {
	case AttrSubtotalWon:
		return SubReasonMinSpend
	case AttrLineCategory:
		return SubReasonCategory
	case AttrLineBrandCode:
		return SubReasonBrand
	case AttrLineOnSale:
		return SubReasonOnSale
	default:
		return SubReasonCondition
	}
}

func lineSubReason(linePreds, excludePreds []Predicate) string {
	for _, p := range linePreds {
		if sub := subReasonFor(p); sub != SubReasonCondition {
			return sub
		}
	}
	for _, p := range excludePreds {
		if p.Attr == AttrLineOnSale {
			return SubReasonOnSale
		}
	}
	return SubReasonNoLineMatch
}

func skuIDsOf(lines []EvaluationLine) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.SkuID != "" {
			out = append(out, line.SkuID)
		}
	}
	sort.Strings(out)
	return out
}

// matchScalar evaluates a cart- or customer-level predicate. The second return
// is false when the context cannot answer the attribute at all, which callers
// treat as a failed match rather than a pass.
func (p Predicate) matchScalar(ctx EvaluationContext) (ok bool, known bool) {
	switch p.Attr {
	case AttrSubtotalWon:
		return p.compareNumber(float64(ctx.SubtotalWon())), true
	case AttrShippingFeeWon:
		return p.compareNumber(float64(ctx.ShippingFeeWon)), true
	case AttrItemCount:
		return p.compareNumber(float64(ctx.itemCount())), true
	case AttrCustomerPaidOrderCount:
		if ctx.PaidOrderCount == nil {
			// Unknown history fails closed: a first-order-only code must not
			// pay out just because the caller could not look the count up.
			return false, false
		}
		return p.compareNumber(float64(*ctx.PaidOrderCount)), true
	}
	return false, false
}

func (p Predicate) matchLine(line EvaluationLine, _ EvaluationContext) (ok bool, known bool) {
	switch p.Attr {
	case AttrLineCategory:
		return p.compareString(line.Category), true
	case AttrLineBrandCode:
		return p.compareString(line.BrandCode), true
	case AttrLineSkuID:
		return p.compareString(line.SkuID), true
	case AttrLineProductID:
		return p.compareString(line.ProductID), true
	case AttrLineUnitPriceWon:
		return p.compareNumber(float64(line.UnitPriceWon)), true
	case AttrLineOnSale:
		return p.compareBool(line.OnSale), true
	}
	return false, false
}

func (p Predicate) compareNumber(actual float64) bool {
	if p.Op.isSetOp() {
		found := false
		for _, v := range toList(p.Value) {
			if n, ok := toNumber(v); ok && n == actual {
				found = true
				break
			}
		}
		return (p.Op == OpIn) == found
	}
	want, ok := toNumber(p.Value)
	if !ok {
		return false
	}
	switch p.Op {
	case OpEq:
		return actual == want
	case OpNeq:
		return actual != want
	case OpGte:
		return actual >= want
	case OpLte:
		return actual <= want
	case OpGt:
		return actual > want
	case OpLt:
		return actual < want
	}
	return false
}

func (p Predicate) compareString(actual string) bool {
	norm := func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
	a := norm(actual)
	if p.Op.isSetOp() {
		found := false
		for _, v := range toList(p.Value) {
			if s, ok := v.(string); ok && norm(s) == a {
				found = true
				break
			}
		}
		return (p.Op == OpIn) == found
	}
	want, ok := p.Value.(string)
	if !ok {
		return false
	}
	switch p.Op {
	case OpEq:
		return a == norm(want)
	case OpNeq:
		return a != norm(want)
	}
	return false
}

func (p Predicate) compareBool(actual bool) bool {
	want, ok := p.Value.(bool)
	if !ok {
		return false
	}
	switch p.Op {
	case OpEq:
		return actual == want
	case OpNeq:
		return actual != want
	}
	return false
}

// toNumber accepts the shapes a JSONB round-trip can produce for a number.
func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func toList(v any) []any {
	if list, ok := v.([]any); ok {
		return list
	}
	return nil
}

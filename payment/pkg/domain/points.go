package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// Points (적립금) are dupli1's store credit: 1 P is ₩1 at checkout.
// See docs/payment-points.md for the rules this file implements.
//
// The ledger is a set of lots per customer. A lot is points that arrived
// together (one order's earning, one review reward, one manager grant) and
// expire together, 12 months after they were credited. Spending takes from
// the lots that expire soonest. A hold is points set aside for one order at
// checkout; it is consumed when the order is paid and released (the points go
// back to the lots they came from) when it is canceled.

var (
	ErrInvalidPoints      = errors.New("invalid points request")
	ErrInsufficientPoints = errors.New("not enough points")
	// ErrPointsHoldConflict: the order already holds a different amount.
	ErrPointsHoldConflict = errors.New("order already holds a different amount of points")
	// ErrPointsHoldClosed: the hold was already consumed or released and
	// cannot change state that way.
	ErrPointsHoldClosed   = errors.New("points hold is no longer open")
	ErrPointsHoldNotFound = errors.New("points hold not found")
)

// PointsValidity is how long credited points stay spendable.
const PointsValidityMonths = 12

// BaseEarnBps is the base earning rate in basis points: 100 = 1% of goods paid.
const BaseEarnBps = 100

// Lot sources. A lot is unique per (source, ref), which is what makes every
// credit idempotent: a replayed event finds its lot and does nothing.
const (
	PointSourceOrder  = "order"  // ref = order id
	PointSourceReview = "review" // ref = review id (granted by a later change)
	PointSourceAdjust = "adjust" // ref = manager adjustment id
)

// Lot statuses.
const (
	LotPending   = "pending"   // order paid, not yet fulfilled: shown, not spendable
	LotAvailable = "available" // spendable while Remaining > 0 and not past ExpiresAt
	LotExpired   = "expired"   // passed ExpiresAt; whatever remained is gone
	LotRevoked   = "revoked"   // order canceled or refunded, review hidden, …
)

// Hold statuses.
const (
	HoldReserved = "reserved"
	HoldConsumed = "consumed"
	HoldReleased = "released"
)

// Entry kinds — the shopper-visible history.
const (
	EntryEarn     = "earn"     // order fulfilled
	EntryGrant    = "grant"    // review reward, manager grant
	EntrySpend    = "spend"    // used on a paid order
	EntryReturn   = "return"   // a paid order that used points was canceled
	EntryClawback = "clawback" // points earned on an order taken back after a refund
	EntryAdjust   = "adjust"   // a manager took points away
	EntryExpire   = "expire"   // reached the end of the 12 months
)

// PointLot is points that arrived together and expire together.
type PointLot struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Source     string `json:"source"`
	Ref        string `json:"ref"`
	Status     string `json:"status"`
	Points     int64  `json:"points"`
	Remaining  int64  `json:"remaining"`
	// BaseWon is the goods amount an order lot was earned on (after discounts,
	// without shipping, card surcharge or the part paid with points). It is
	// what counts toward the VIP tier. Zero for non-order lots.
	BaseWon int64 `json:"base_won,omitempty"`
	// Multiplier is the tier multiplier in tenths (10 = 1×) the lot was
	// earned at, recorded so the history can say "Gold 2×".
	Multiplier int `json:"multiplier,omitempty"`
	// ExpiredPoints is how much of the lot was lost to expiry.
	ExpiredPoints int64      `json:"expired_points,omitempty"`
	Note          string     `json:"note,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	CreditedAt    *time.Time `json:"credited_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

// PointAllocation is how much of one lot a hold took.
type PointAllocation struct {
	LotID  string `json:"lot_id"`
	Points int64  `json:"points"`
}

// PointHold is points set aside for one order.
type PointHold struct {
	OrderID     string            `json:"order_id"`
	CustomerID  string            `json:"customer_id"`
	Points      int64             `json:"points"`
	Status      string            `json:"status"`
	Allocations []PointAllocation `json:"allocations"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// PointEntry is one line of a customer's points history. Points is signed.
type PointEntry struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Kind       string    `json:"kind"`
	Points     int64     `json:"points"`
	OrderID    string    `json:"order_id,omitempty"`
	Ref        string    `json:"ref,omitempty"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// PointAccount is everything the ledger knows about one customer, loaded and
// changed as a unit under the customer's lock. NewEntries collects the history
// lines a mutation adds; the store appends them and clears the slice.
type PointAccount struct {
	CustomerID string
	Lots       []*PointLot
	Holds      []*PointHold
	NewEntries []PointEntry

	newID func() string
}

// NewPointID mints an opaque id for a lot or an entry.
func NewPointID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "pt_" + hex.EncodeToString(b[:])
}

// NewPointAccount wraps a loaded account; newID mints ids for new lots and entries.
func NewPointAccount(customerID string, lots []*PointLot, holds []*PointHold, newID func() string) *PointAccount {
	return &PointAccount{CustomerID: customerID, Lots: lots, Holds: holds, newID: newID}
}

// Tier is a VIP level. Multiplier is in tenths (15 = 1.5×).
type Tier struct {
	Code        string `json:"code"`
	Multiplier  int    `json:"multiplier"`
	MinSpendWon int64  `json:"min_spend_won"`
}

// Tiers, lowest first. Spend is goods paid on fulfilled orders over the 12
// calendar months (KST) before the current month, so a tier holds for the
// whole month and is rechecked on the 1st.
var Tiers = []Tier{
	{Code: "member", Multiplier: 10, MinSpendWon: 0},
	{Code: "silver", Multiplier: 15, MinSpendWon: 1_000_000},
	{Code: "gold", Multiplier: 20, MinSpendWon: 3_000_000},
	{Code: "diamond", Multiplier: 30, MinSpendWon: 5_000_000},
}

// TierStatus is a customer's tier with what it was decided on.
type TierStatus struct {
	Tier
	SpendWon   int64     `json:"spend_won"`
	WindowFrom time.Time `json:"window_from"`
	WindowTo   time.Time `json:"window_to"`
	Next       *Tier     `json:"next,omitempty"`
}

// TierWindow is the spend window that decides the tier in force at `at`:
// the 12 KST calendar months ending where the current month starts.
func TierWindow(at time.Time) (from, to time.Time) {
	k := at.In(reportperiod.Location)
	to = time.Date(k.Year(), k.Month(), 1, 0, 0, 0, 0, reportperiod.Location)
	from = to.AddDate(0, -12, 0)
	return from.UTC(), to.UTC()
}

// TierFor picks the highest tier whose threshold spend reaches.
func TierFor(spendWon int64) (Tier, *Tier) {
	cur := Tiers[0]
	var next *Tier
	for i, t := range Tiers {
		if spendWon >= t.MinSpendWon {
			cur = t
			next = nil
			if i+1 < len(Tiers) {
				n := Tiers[i+1]
				next = &n
			}
		}
	}
	return cur, next
}

// EarnPoints is what basewon earns at a multiplier, rounded down.
func EarnPoints(baseWon int64, multiplier int) int64 {
	if baseWon <= 0 || multiplier <= 0 {
		return 0
	}
	return baseWon * BaseEarnBps * int64(multiplier) / (10000 * 10)
}

// TierAt is the account's tier in force at `at`.
func (a *PointAccount) TierAt(at time.Time) TierStatus {
	from, to := TierWindow(at)
	var spend int64
	for _, l := range a.Lots {
		if l.Source != PointSourceOrder || l.CreditedAt == nil {
			continue
		}
		if l.Status != LotAvailable && l.Status != LotExpired {
			continue
		}
		if !l.CreditedAt.Before(from) && l.CreditedAt.Before(to) {
			spend += l.BaseWon
		}
	}
	tier, next := TierFor(spend)
	return TierStatus{Tier: tier, SpendWon: spend, WindowFrom: from, WindowTo: to, Next: next}
}

// Balance is the spendable points at `at`: what remains in available lots
// that have not expired. Points held for a checkout are already out of it.
func (a *PointAccount) Balance(at time.Time) int64 {
	var n int64
	for _, l := range a.Lots {
		if spendable(l, at) {
			n += l.Remaining
		}
	}
	return n
}

// Pending is points earned on paid orders that are not yet fulfilled.
func (a *PointAccount) Pending() int64 {
	var n int64
	for _, l := range a.Lots {
		if l.Status == LotPending {
			n += l.Points
		}
	}
	return n
}

// ExpiringBy is spendable points that expire on or before `by`.
func (a *PointAccount) ExpiringBy(at, by time.Time) int64 {
	var n int64
	for _, l := range a.Lots {
		if spendable(l, at) && l.ExpiresAt != nil && !l.ExpiresAt.After(by) {
			n += l.Remaining
		}
	}
	return n
}

func spendable(l *PointLot, at time.Time) bool {
	return l.Status == LotAvailable && l.Remaining > 0 && l.ExpiresAt != nil && l.ExpiresAt.After(at)
}

func (a *PointAccount) lot(source, ref string) *PointLot {
	for _, l := range a.Lots {
		if l.Source == source && l.Ref == ref {
			return l
		}
	}
	return nil
}

func (a *PointAccount) lotByID(id string) *PointLot {
	for _, l := range a.Lots {
		if l.ID == id {
			return l
		}
	}
	return nil
}

// Hold returns the order's hold, or nil.
func (a *PointAccount) Hold(orderID string) *PointHold {
	for _, h := range a.Holds {
		if h.OrderID == orderID {
			return h
		}
	}
	return nil
}

func (a *PointAccount) entry(kind string, points int64, orderID, ref, note string, at time.Time) {
	a.NewEntries = append(a.NewEntries, PointEntry{
		ID: a.newID(), CustomerID: a.CustomerID, Kind: kind, Points: points,
		OrderID: orderID, Ref: ref, Note: note, CreatedAt: at,
	})
}

func expiry(at time.Time) *time.Time {
	e := at.AddDate(0, PointsValidityMonths, 0)
	return &e
}

// pointsUsedOn is what the order paid with points: the hold, unless it was
// released.
func (a *PointAccount) pointsUsedOn(orderID string) int64 {
	if h := a.Hold(orderID); h != nil && h.Status != HoldReleased {
		return h.Points
	}
	return 0
}

// earnBase is the goods amount an order earns on: goods after discounts,
// less the part paid with points (points do not earn points).
func (a *PointAccount) earnBase(orderID string, goodsWon int64) int64 {
	base := goodsWon - a.pointsUsedOn(orderID)
	if base < 0 {
		return 0
	}
	return base
}

// OrderPaid records the points an order will earn as pending, at the tier in
// force now. It is an estimate the shopper sees; OrderFulfilled settles it.
// goodsWon is the goods subtotal after discounts.
func (a *PointAccount) OrderPaid(orderID string, goodsWon int64, at time.Time) {
	if orderID == "" || a.lot(PointSourceOrder, orderID) != nil {
		return
	}
	base := a.earnBase(orderID, goodsWon)
	tier := a.TierAt(at)
	a.Lots = append(a.Lots, &PointLot{
		ID: a.newID(), CustomerID: a.CustomerID, Source: PointSourceOrder, Ref: orderID,
		Status: LotPending, Points: EarnPoints(base, tier.Multiplier), BaseWon: base,
		Multiplier: tier.Multiplier, CreatedAt: at,
	})
}

// OrderFulfilled credits the order's points at the tier in force now. A
// missed paid event is covered: the lot is created here. Replays do nothing.
func (a *PointAccount) OrderFulfilled(orderID string, goodsWon int64, at time.Time) {
	if orderID == "" {
		return
	}
	l := a.lot(PointSourceOrder, orderID)
	if l == nil {
		a.OrderPaid(orderID, goodsWon, at)
		l = a.lot(PointSourceOrder, orderID)
	}
	if l.Status != LotPending {
		return
	}
	tier := a.TierAt(at)
	l.Multiplier = tier.Multiplier
	l.Points = EarnPoints(l.BaseWon, tier.Multiplier)
	l.Remaining = l.Points
	l.Status = LotAvailable
	credited := at
	l.CreditedAt = &credited
	l.ExpiresAt = expiry(at)
	if l.Points > 0 {
		a.entry(EntryEarn, l.Points, orderID, "", "", at)
	}
}

// OrderCanceled undoes everything points did for an order: the pending
// earning is dropped, credited points are taken back, and points the order
// paid with are returned. It returns how much of a clawback could not be
// collected because the customer had already spent it.
func (a *PointAccount) OrderCanceled(orderID string, at time.Time) (shortfall int64) {
	if orderID == "" {
		return 0
	}
	if l := a.lot(PointSourceOrder, orderID); l != nil {
		switch l.Status {
		case LotPending:
			l.Status = LotRevoked
		case LotAvailable, LotExpired:
			shortfall = a.clawback(l, orderID, at)
		}
	}
	if h := a.Hold(orderID); h != nil && h.Status != HoldReleased {
		_ = a.Release(orderID, at)
	}
	return shortfall
}

// clawback takes a credited lot back: what is left in it, then the rest from
// the customer's other lots, soonest-expiring first. What cannot be found is
// returned as the shortfall and not carried as debt.
func (a *PointAccount) clawback(l *PointLot, orderID string, at time.Time) int64 {
	want := l.Points
	if l.Status == LotExpired {
		// Expired points were never spendable after expiry; only what was
		// spent from the lot before then is still owed.
		want = l.Points - l.ExpiredPoints
	}
	taken := int64(0)
	if l.Status == LotAvailable {
		taken = min64(l.Remaining, want)
		l.Remaining -= taken
	}
	l.Status = LotRevoked
	if rest := want - taken; rest > 0 {
		taken += a.deduct(rest, at)
	}
	if taken > 0 {
		a.entry(EntryClawback, -taken, orderID, "", "", at)
	}
	return want - taken
}

// deduct removes up to n points from spendable lots, soonest-expiring first,
// and returns how many it removed.
func (a *PointAccount) deduct(n int64, at time.Time) int64 {
	var took int64
	for _, l := range a.spendableLots(at) {
		if n == 0 {
			break
		}
		t := min64(l.Remaining, n)
		l.Remaining -= t
		n -= t
		took += t
	}
	return took
}

// spendableLots lists lots that can be spent, soonest-expiring first.
func (a *PointAccount) spendableLots(at time.Time) []*PointLot {
	var out []*PointLot
	for _, l := range a.Lots {
		if spendable(l, at) {
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ExpiresAt.Equal(*out[j].ExpiresAt) {
			return out[i].ExpiresAt.Before(*out[j].ExpiresAt)
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Grant adds points that do not come from an order (a review reward, a
// manager grant). Idempotent per (source, ref): a second call returns false.
func (a *PointAccount) Grant(source, ref string, points int64, note string, at time.Time) (bool, error) {
	if source == "" || ref == "" || points <= 0 || source == PointSourceOrder {
		return false, ErrInvalidPoints
	}
	if a.lot(source, ref) != nil {
		return false, nil
	}
	credited := at
	a.Lots = append(a.Lots, &PointLot{
		ID: a.newID(), CustomerID: a.CustomerID, Source: source, Ref: ref,
		Status: LotAvailable, Points: points, Remaining: points, Note: note,
		CreatedAt: at, CreditedAt: &credited, ExpiresAt: expiry(at),
	})
	a.entry(EntryGrant, points, "", ref, note, at)
	return true, nil
}

// Deduct takes points away (a manager correction). It never goes below zero:
// asking for more than the balance is ErrInsufficientPoints.
func (a *PointAccount) Deduct(ref string, points int64, note string, at time.Time) error {
	if points <= 0 {
		return ErrInvalidPoints
	}
	if a.Balance(at) < points {
		return ErrInsufficientPoints
	}
	a.deduct(points, at)
	a.entry(EntryAdjust, -points, "", ref, note, at)
	return nil
}

// Reserve sets points aside for an order at checkout. Calling it again for
// the same order and amount returns the existing hold; a different amount is
// ErrPointsHoldConflict.
func (a *PointAccount) Reserve(orderID string, points int64, at time.Time) (*PointHold, error) {
	if orderID == "" || points <= 0 {
		return nil, ErrInvalidPoints
	}
	if h := a.Hold(orderID); h != nil {
		if h.Points == points && h.Status != HoldReleased {
			return h, nil
		}
		return nil, ErrPointsHoldConflict
	}
	if a.Balance(at) < points {
		return nil, ErrInsufficientPoints
	}
	h := &PointHold{OrderID: orderID, CustomerID: a.CustomerID, Points: points, Status: HoldReserved, CreatedAt: at, UpdatedAt: at}
	need := points
	for _, l := range a.spendableLots(at) {
		if need == 0 {
			break
		}
		t := min64(l.Remaining, need)
		l.Remaining -= t
		need -= t
		h.Allocations = append(h.Allocations, PointAllocation{LotID: l.ID, Points: t})
	}
	a.Holds = append(a.Holds, h)
	return h, nil
}

// Consume marks a hold spent once the order is paid. Idempotent.
func (a *PointAccount) Consume(orderID string, at time.Time) (*PointHold, error) {
	h := a.Hold(orderID)
	if h == nil {
		return nil, ErrPointsHoldNotFound
	}
	switch h.Status {
	case HoldConsumed:
		return h, nil
	case HoldReleased:
		return nil, ErrPointsHoldClosed
	}
	h.Status = HoldConsumed
	h.UpdatedAt = at
	a.entry(EntrySpend, -h.Points, orderID, "", "", at)
	return h, nil
}

// Release gives a hold's points back to the lots they came from, keeping
// those lots' expiry dates: a lot that expired in the meantime expires again
// at once. Idempotent.
func (a *PointAccount) Release(orderID string, at time.Time) error {
	h := a.Hold(orderID)
	if h == nil {
		return ErrPointsHoldNotFound
	}
	if h.Status == HoldReleased {
		return nil
	}
	wasSpent := h.Status == HoldConsumed
	var lapsed int64
	for _, al := range h.Allocations {
		l := a.lotByID(al.LotID)
		if l == nil {
			continue
		}
		switch {
		case l.Status == LotRevoked:
			// The lot itself was taken back; the points go nowhere.
		case l.ExpiresAt != nil && !l.ExpiresAt.After(at):
			l.Status = LotExpired
			l.ExpiredPoints += al.Points
			lapsed += al.Points
		default:
			l.Remaining += al.Points
		}
	}
	h.Status = HoldReleased
	h.UpdatedAt = at
	if wasSpent {
		a.entry(EntryReturn, h.Points, orderID, "", "", at)
	}
	if lapsed > 0 {
		a.entry(EntryExpire, -lapsed, orderID, "", "returned after expiry", at)
	}
	return nil
}

// Expire ends lots past their date and records what was lost. It returns the
// number of points expired.
func (a *PointAccount) Expire(at time.Time) int64 {
	var total int64
	for _, l := range a.Lots {
		if l.Status != LotAvailable || l.ExpiresAt == nil || l.ExpiresAt.After(at) {
			continue
		}
		l.Status = LotExpired
		if l.Remaining > 0 {
			l.ExpiredPoints += l.Remaining
			total += l.Remaining
			a.entry(EntryExpire, -l.Remaining, "", l.Ref, "", at)
			l.Remaining = 0
		}
	}
	return total
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func newAccount() *PointAccount {
	n := 0
	return NewPointAccount("cust-1", nil, nil, func() string { n++; return fmt.Sprintf("id-%d", n) })
}

var t0 = time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC) // 12:00 KST

func TestEarnPoints(t *testing.T) {
	cases := []struct {
		base int64
		mult int
		want int64
	}{
		{100_000, 10, 1_000},
		{100_000, 15, 1_500},
		{100_000, 20, 2_000},
		{100_000, 30, 3_000},
		{99, 10, 0},
		{12_345, 15, 185}, // 185.175 rounds down
		{0, 10, 0},
	}
	for _, c := range cases {
		if got := EarnPoints(c.base, c.mult); got != c.want {
			t.Errorf("EarnPoints(%d, %d) = %d, want %d", c.base, c.mult, got, c.want)
		}
	}
}

func TestTierFor(t *testing.T) {
	cases := []struct {
		spend int64
		code  string
		next  string
	}{
		{0, "member", "silver"},
		{999_999, "member", "silver"},
		{1_000_000, "silver", "gold"},
		{3_000_000, "gold", "diamond"},
		{5_000_000, "diamond", ""},
		{9_000_000, "diamond", ""},
	}
	for _, c := range cases {
		tier, next := TierFor(c.spend)
		if tier.Code != c.code {
			t.Errorf("TierFor(%d) = %s, want %s", c.spend, tier.Code, c.code)
		}
		gotNext := ""
		if next != nil {
			gotNext = next.Code
		}
		if gotNext != c.next {
			t.Errorf("TierFor(%d) next = %q, want %q", c.spend, gotNext, c.next)
		}
	}
}

func TestTierWindowIsTwelveKSTMonthsBeforeThisMonth(t *testing.T) {
	// 2026-11-30 16:00 UTC is already 2026-12-01 01:00 in KST.
	from, to := TierWindow(time.Date(2026, 11, 30, 16, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 11, 30, 15, 0, 0, 0, time.UTC); !to.Equal(want) {
		t.Fatalf("to = %s, want %s", to, want)
	}
	if want := time.Date(2025, 11, 30, 15, 0, 0, 0, time.UTC); !from.Equal(want) {
		t.Fatalf("from = %s, want %s", from, want)
	}
}

func TestOrderLifecycleEarnsOnFulfillment(t *testing.T) {
	a := newAccount()
	a.OrderPaid("o1", 250_000, t0)
	if got := a.Pending(); got != 2_500 {
		t.Fatalf("pending = %d, want 2500", got)
	}
	if got := a.Balance(t0); got != 0 {
		t.Fatalf("balance before fulfillment = %d, want 0", got)
	}
	a.OrderFulfilled("o1", 250_000, t0.Add(time.Hour))
	if got := a.Balance(t0.Add(time.Hour)); got != 2_500 {
		t.Fatalf("balance = %d, want 2500", got)
	}
	if a.Pending() != 0 {
		t.Fatalf("pending after fulfillment = %d", a.Pending())
	}
	// Replays change nothing.
	a.OrderPaid("o1", 250_000, t0)
	a.OrderFulfilled("o1", 250_000, t0)
	if got := a.Balance(t0.Add(time.Hour)); got != 2_500 {
		t.Fatalf("balance after replay = %d", got)
	}
	if n := len(a.NewEntries); n != 1 || a.NewEntries[0].Kind != EntryEarn {
		t.Fatalf("entries = %+v, want one earn", a.NewEntries)
	}
}

func TestFulfilledWithoutPaidEventStillEarns(t *testing.T) {
	a := newAccount()
	a.OrderFulfilled("o1", 100_000, t0)
	if got := a.Balance(t0); got != 1_000 {
		t.Fatalf("balance = %d, want 1000", got)
	}
}

func TestCancelBeforeFulfillmentDropsPending(t *testing.T) {
	a := newAccount()
	a.OrderPaid("o1", 100_000, t0)
	a.OrderCanceled("o1", t0)
	a.OrderFulfilled("o1", 100_000, t0) // a late event must not resurrect it
	if a.Pending() != 0 || a.Balance(t0) != 0 {
		t.Fatalf("pending %d balance %d, want 0/0", a.Pending(), a.Balance(t0))
	}
}

func TestTierMultiplierAppliesAtFulfillment(t *testing.T) {
	a := newAccount()
	// ₩3,200,000 fulfilled in October 2026 makes the customer Gold for November.
	oct := time.Date(2026, 10, 10, 3, 0, 0, 0, time.UTC)
	a.OrderFulfilled("big", 3_200_000, oct)
	if tier := a.TierAt(t0); tier.Code != "gold" || tier.SpendWon != 3_200_000 {
		t.Fatalf("tier = %+v, want gold on 3,200,000", tier)
	}
	// In October itself the order did not count yet.
	if tier := a.TierAt(oct); tier.Code != "member" {
		t.Fatalf("tier in October = %s, want member", tier.Code)
	}
	a.OrderFulfilled("next", 100_000, t0)
	l := a.lot(PointSourceOrder, "next")
	if l.Points != 2_000 || l.Multiplier != 20 {
		t.Fatalf("next order lot = %+v, want 2000 at 2x", l)
	}
}

func TestReserveConsumeReleaseKeepsExpiry(t *testing.T) {
	a := newAccount()
	if _, err := a.Grant(PointSourceAdjust, "g1", 5_000, "welcome", t0); err != nil {
		t.Fatal(err)
	}
	later := t0.AddDate(0, 1, 0)
	if _, err := a.Grant(PointSourceAdjust, "g2", 5_000, "", later); err != nil {
		t.Fatal(err)
	}
	at := later.Add(time.Hour)
	h, err := a.Reserve("o1", 7_000, at)
	if err != nil {
		t.Fatal(err)
	}
	// Soonest-expiring first: all of g1, then 2,000 of g2.
	if len(h.Allocations) != 2 || h.Allocations[0].Points != 5_000 || h.Allocations[1].Points != 2_000 {
		t.Fatalf("allocations = %+v", h.Allocations)
	}
	if got := a.Balance(at); got != 3_000 {
		t.Fatalf("balance while held = %d, want 3000", got)
	}
	if _, err := a.Reserve("o1", 7_000, at); err != nil {
		t.Fatalf("same reserve again: %v", err)
	}
	if _, err := a.Reserve("o1", 6_000, at); !errors.Is(err, ErrPointsHoldConflict) {
		t.Fatalf("different amount err = %v", err)
	}
	if _, err := a.Consume("o1", at); err != nil {
		t.Fatal(err)
	}
	if err := a.Release("o1", at); err != nil {
		t.Fatal(err)
	}
	if got := a.Balance(at); got != 10_000 {
		t.Fatalf("balance after return = %d, want 10000", got)
	}
	kinds := []string{}
	for _, e := range a.NewEntries {
		kinds = append(kinds, e.Kind)
	}
	if fmt.Sprint(kinds) != "[grant grant spend return]" {
		t.Fatalf("entries = %v", kinds)
	}
}

func TestReserveRefusesMoreThanBalance(t *testing.T) {
	a := newAccount()
	_, _ = a.Grant(PointSourceAdjust, "g1", 1_000, "", t0)
	if _, err := a.Reserve("o1", 1_001, t0); !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("err = %v, want insufficient", err)
	}
}

func TestReleaseAfterExpiryLapses(t *testing.T) {
	a := newAccount()
	_, _ = a.Grant(PointSourceAdjust, "g1", 1_000, "", t0)
	_, _ = a.Reserve("o1", 1_000, t0)
	_, _ = a.Consume("o1", t0)
	after := t0.AddDate(1, 0, 1)
	_ = a.Release("o1", after)
	if got := a.Balance(after); got != 0 {
		t.Fatalf("balance = %d, want 0", got)
	}
	last := a.NewEntries[len(a.NewEntries)-1]
	if last.Kind != EntryExpire || last.Points != -1_000 {
		t.Fatalf("last entry = %+v, want expire -1000", last)
	}
}

func TestExpire(t *testing.T) {
	a := newAccount()
	_, _ = a.Grant(PointSourceAdjust, "g1", 1_000, "", t0)
	_, _ = a.Reserve("o1", 400, t0)
	_, _ = a.Consume("o1", t0)
	if n := a.Expire(t0.AddDate(1, 0, 0).Add(-time.Second)); n != 0 {
		t.Fatalf("expired early: %d", n)
	}
	if n := a.Expire(t0.AddDate(1, 0, 0)); n != 600 {
		t.Fatalf("expired = %d, want 600", n)
	}
	if n := a.Expire(t0.AddDate(1, 0, 1)); n != 0 {
		t.Fatalf("expired twice: %d", n)
	}
}

func TestPointsUsedDoNotEarn(t *testing.T) {
	a := newAccount()
	_, _ = a.Grant(PointSourceAdjust, "g1", 100_000, "", t0)
	_, _ = a.Reserve("o1", 100_000, t0)
	_, _ = a.Consume("o1", t0)
	a.OrderFulfilled("o1", 300_000, t0)
	if l := a.lot(PointSourceOrder, "o1"); l.BaseWon != 200_000 || l.Points != 2_000 {
		t.Fatalf("lot = %+v, want base 200000 earning 2000", l)
	}
}

func TestCanceledAfterFulfillmentClawsBackAndReturnsSpentPoints(t *testing.T) {
	a := newAccount()
	_, _ = a.Grant(PointSourceAdjust, "g1", 10_000, "", t0)
	_, _ = a.Reserve("o1", 10_000, t0)
	_, _ = a.Consume("o1", t0)
	a.OrderFulfilled("o1", 510_000, t0) // earns 5,000 on 500,000
	if a.Balance(t0) != 5_000 {
		t.Fatalf("balance = %d", a.Balance(t0))
	}
	if short := a.OrderCanceled("o1", t0); short != 0 {
		t.Fatalf("shortfall = %d", short)
	}
	// Earned 5,000 taken back, 10,000 spent returned.
	if got := a.Balance(t0); got != 10_000 {
		t.Fatalf("balance = %d, want 10000", got)
	}
}

func TestClawbackShortfallWhenAlreadySpent(t *testing.T) {
	a := newAccount()
	a.OrderFulfilled("o1", 500_000, t0) // 5,000
	_, _ = a.Reserve("o2", 4_000, t0)
	_, _ = a.Consume("o2", t0)
	if short := a.OrderCanceled("o1", t0); short != 4_000 {
		t.Fatalf("shortfall = %d, want 4000", short)
	}
	if a.Balance(t0) != 0 {
		t.Fatalf("balance = %d, want 0", a.Balance(t0))
	}
}

func TestGrantIdempotentAndDeduct(t *testing.T) {
	a := newAccount()
	if ok, _ := a.Grant(PointSourceReview, "r1", 2_000, "", t0); !ok {
		t.Fatal("first grant refused")
	}
	if ok, _ := a.Grant(PointSourceReview, "r1", 2_000, "", t0); ok {
		t.Fatal("second grant applied")
	}
	if _, err := a.Grant(PointSourceOrder, "x", 1, "", t0); !errors.Is(err, ErrInvalidPoints) {
		t.Fatalf("order grant err = %v", err)
	}
	if err := a.Deduct("adj1", 2_001, "", t0); !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("over-deduct err = %v", err)
	}
	if err := a.Deduct("adj1", 500, "typo", t0); err != nil {
		t.Fatal(err)
	}
	if a.Balance(t0) != 1_500 {
		t.Fatalf("balance = %d", a.Balance(t0))
	}
}

package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

func orderEvent(t *testing.T, status string, subtotal, discount int64, at time.Time) []byte {
	t.Helper()
	b, err := json.Marshal(events.Order{
		OrderID: "o1", CustomerID: "u-1", Status: status,
		SubtotalWon: subtotal, DiscountWon: discount, ShippingFeeWon: 0,
		TotalWon: subtotal - discount, Occurred: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPointsFollowOrderEvents(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	p := service.NewPoints(memory.NewPoints()).WithClock(func() time.Time { return now })

	// ₩300,000 of goods with a ₩45,000 code earns 1% of ₩255,000.
	if err := p.HandleOrderEvent(ctx, events.OrderPaid, orderEvent(t, "paid", 300_000, 45_000, now)); err != nil {
		t.Fatal(err)
	}
	s, err := p.Summary(ctx, "u-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if s.Pending != 2_550 || s.Balance != 0 {
		t.Fatalf("after paid: pending %d balance %d", s.Pending, s.Balance)
	}
	// Statuses that do not touch points are ignored.
	if err := p.HandleOrderEvent(ctx, events.OrderStatusUpdate, orderEvent(t, "in_transit", 300_000, 45_000, now)); err != nil {
		t.Fatal(err)
	}
	if err := p.HandleOrderEvent(ctx, events.OrderStatusUpdate, orderEvent(t, "fulfilled", 300_000, 45_000, now)); err != nil {
		t.Fatal(err)
	}
	s, _ = p.Summary(ctx, "u-1", 10)
	if s.Pending != 0 || s.Balance != 2_550 || len(s.Entries) != 1 {
		t.Fatalf("after fulfilled: %+v", s)
	}
	if err := p.HandleOrderEvent(ctx, events.OrderStatusUpdate, orderEvent(t, "canceled", 300_000, 45_000, now)); err != nil {
		t.Fatal(err)
	}
	s, _ = p.Summary(ctx, "u-1", 10)
	if s.Balance != 0 {
		t.Fatalf("after refund: balance %d", s.Balance)
	}
}

func TestExpireDue(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	clock := now
	p := service.NewPoints(memory.NewPoints()).WithClock(func() time.Time { return clock })
	if _, err := p.Adjust(ctx, service.AdjustInput{CustomerID: "u-1", Points: 1_000, Reason: "goodwill"}); err != nil {
		t.Fatal(err)
	}
	clock = now.AddDate(1, 0, 0)
	n, err := p.ExpireDue(ctx)
	if err != nil || n != 1 {
		t.Fatalf("ExpireDue = %d, %v", n, err)
	}
	s, _ := p.Summary(ctx, "u-1", 10)
	if s.Balance != 0 || s.Entries[0].Kind != "expire" {
		t.Fatalf("after expiry: %+v", s)
	}
}

func reviewEvent(t *testing.T, action string, bodyChars, photos int, hidden bool, at time.Time) []byte {
	t.Helper()
	b, err := json.Marshal(events.ProductReview{
		Action: action, ReviewID: "rev-1", ProductID: "BAG-1", SkuID: "sku-1", OrderID: "o1",
		CustomerID: "u-1", Rating: 5, BodyChars: bodyChars, PhotoCount: photos, Hidden: hidden, Occurred: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPointsFollowReviewEvents(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC)
	p := service.NewPoints(memory.NewPoints()).WithClock(func() time.Time { return now })
	balance := func() int64 {
		s, err := p.Summary(ctx, "u-1", 10)
		if err != nil {
			t.Fatal(err)
		}
		return s.Balance
	}

	steps := []struct {
		name  string
		event []byte
		want  int64
	}{
		{"short text", reviewEvent(t, events.ReviewCreated, 12, 0, false, now), 0},
		{"edited to 20+", reviewEvent(t, events.ReviewUpdated, 24, 0, false, now), 2_000},
		{"replayed", reviewEvent(t, events.ReviewUpdated, 24, 0, false, now), 2_000},
		{"photo added", reviewEvent(t, events.ReviewUpdated, 24, 1, false, now), 5_000},
		{"hidden", reviewEvent(t, events.ReviewHidden, 24, 1, true, now), 0},
		{"unhidden", reviewEvent(t, events.ReviewUnhidden, 24, 1, false, now), 0},
	}
	for _, step := range steps {
		if err := p.HandleReviewEvent(ctx, events.ProductReviewChanged, step.event); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if got := balance(); got != step.want {
			t.Fatalf("%s: balance %d, want %d", step.name, got, step.want)
		}
	}
}

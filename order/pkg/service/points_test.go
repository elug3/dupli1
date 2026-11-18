package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

// fakePoints is payment's points ledger as order sees it: holds by order id.
type fakePoints struct {
	balance  int64
	reserved map[string]int64
	consumed []string
	released []string
	err      error
}

func newFakePoints(balance int64) *fakePoints {
	return &fakePoints{balance: balance, reserved: map[string]int64{}}
}

func (f *fakePoints) Reserve(_ context.Context, orderID, _ string, points int64) error {
	if f.err != nil {
		return f.err
	}
	if points > f.balance {
		return ports.ErrInsufficientPoints
	}
	f.balance -= points
	f.reserved[orderID] = points
	return nil
}

func (f *fakePoints) Consume(_ context.Context, orderID string) error {
	f.consumed = append(f.consumed, orderID)
	return nil
}

func (f *fakePoints) Release(_ context.Context, orderID string) error {
	f.released = append(f.released, orderID)
	f.balance += f.reserved[orderID]
	delete(f.reserved, orderID)
	return nil
}

func pointsService(points ports.PointsClient, shippingFee int64) (*service.Service, *fakePayment) {
	pay := &fakePayment{}
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{reservationID: "res-1"}, nil, 0).
		WithProduct(&fakeProduct{defaultKRW: 10000}).
		WithShippingFee(shippingFee).
		WithCardSurcharge(1000).
		WithPayment(pay)
	if points != nil {
		svc = svc.WithPoints(points)
	}
	return svc, pay
}

// ₩10,000 of goods, 4,000 P, ₩3,000 delivery: the card pays 6,000 + 3,000 =
// 9,000, plus 10% on that = 9,900. The surcharge never falls on the points.
func TestCompleteCheckout_PointsComeOffBeforeSurcharge(t *testing.T) {
	ctx := t.Context()
	points := newFakePoints(5000)
	svc, _ := pointsService(points, 3000)
	session := openSessionWithItem(t, svc)

	input := testCompleteCheckoutInput()
	input.PointsWon = 4000
	result, err := svc.CompleteCheckout(ctx, session.ID, input)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	o := result.Order
	if o.PointsWon != 4000 || o.CardSurchargeWon != 900 || o.TotalWon != 9900 {
		t.Fatalf("points/surcharge/total = %d/%d/%d, want 4000/900/9900", o.PointsWon, o.CardSurchargeWon, o.TotalWon)
	}
	if points.reserved[o.ID] != 4000 {
		t.Fatalf("reserved = %v, want 4000 on %s", points.reserved, o.ID)
	}

	if _, err := svc.MarkOrderPaid(ctx, o.ID, "pay-1", 9900); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	if len(points.consumed) != 1 || points.consumed[0] != o.ID {
		t.Fatalf("consumed = %v", points.consumed)
	}
}

// "Use all" sends the whole balance; only what the goods are worth is used,
// and delivery is still paid by card.
func TestCompleteCheckout_PointsTrimmedToGoods(t *testing.T) {
	points := newFakePoints(50_000)
	svc, _ := pointsService(points, 3000)
	session := openSessionWithItem(t, svc)

	input := testCompleteCheckoutInput()
	input.PointsWon = 50_000
	result, err := svc.CompleteCheckout(t.Context(), session.ID, input)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	o := result.Order
	if o.PointsWon != 10000 || o.TotalWon != 3300 {
		t.Fatalf("points/total = %d/%d, want 10000/3300", o.PointsWon, o.TotalWon)
	}
}

// Too few points rolls the order back and releases its stock.
func TestCompleteCheckout_InsufficientPointsRollsBack(t *testing.T) {
	ctx := t.Context()
	points := newFakePoints(1000)
	stock := &fakeStock{reservationID: "res-1"}
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, stock, nil, 0).
		WithProduct(&fakeProduct{defaultKRW: 10000}).
		WithPoints(points)
	session := openSessionWithItem(t, svc)

	input := testCompleteCheckoutInput()
	input.PointsWon = 2000
	if _, err := svc.CompleteCheckout(ctx, session.ID, input); !errors.Is(err, ports.ErrInsufficientPoints) {
		t.Fatalf("err = %v, want ErrInsufficientPoints", err)
	}
	if stock.released != "res-1" {
		t.Fatalf("stock released = %q, want res-1", stock.released)
	}
	orders, _ := svc.ListAllOrders(ctx)
	if len(orders) != 1 || orders[0].Status != domain.StatusCanceled {
		t.Fatalf("orders = %+v, want one canceled", orders)
	}
	// The session stays open, so the shopper can try again with fewer points.
	if s, err := svc.GetCheckoutSession(ctx, session.ID); err != nil || s.Status != domain.CheckoutStatusOpen {
		t.Fatalf("session = %+v, %v", s, err)
	}
}

func TestCompleteCheckout_PointsNeedTheLedger(t *testing.T) {
	svc, _ := pointsService(nil, 0)
	session := openSessionWithItem(t, svc)
	input := testCompleteCheckoutInput()
	input.PointsWon = 100
	if _, err := svc.CompleteCheckout(t.Context(), session.ID, input); !errors.Is(err, ports.ErrPointsUnavailable) {
		t.Fatalf("err = %v, want ErrPointsUnavailable", err)
	}
	input.PointsWon = -1
	if _, err := svc.CompleteCheckout(t.Context(), session.ID, input); !errors.Is(err, ports.ErrInvalidPoints) {
		t.Fatalf("err = %v, want ErrInvalidPoints", err)
	}
}

// Paid entirely in points with free delivery: a ₩0 order, no card surcharge,
// and a cancel gives the points back without asking payment for a refund.
func TestPointsOnlyOrderCancelReturnsPoints(t *testing.T) {
	ctx := t.Context()
	points := newFakePoints(10000)
	svc, pay := pointsService(points, 0)
	session := openSessionWithItem(t, svc)

	input := testCompleteCheckoutInput()
	input.PointsWon = 10000
	result, err := svc.CompleteCheckout(ctx, session.ID, input)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	o := result.Order
	if o.TotalWon != 0 || o.CardSurchargeWon != 0 {
		t.Fatalf("total/surcharge = %d/%d, want 0/0", o.TotalWon, o.CardSurchargeWon)
	}
	if _, err := svc.MarkOrderPaid(ctx, o.ID, "pay-points", 0); err != nil {
		t.Fatalf("MarkOrderPaid ₩0: %v", err)
	}
	if _, err := svc.CancelOrder(ctx, o.ID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if len(pay.canceled) != 0 {
		t.Fatalf("refunded %v, want no card refund", pay.canceled)
	}
	if points.balance != 10000 || len(points.released) == 0 {
		t.Fatalf("balance %d released %v, want points back", points.balance, points.released)
	}
}

// Points are money: a cancel after shipping keeps the stock committed and the
// code spent, but still gives the points back.
func TestCancelAfterShipReturnsPoints(t *testing.T) {
	ctx := t.Context()
	points := newFakePoints(5000)
	svc, pay := pointsService(points, 0)
	session := openSessionWithItem(t, svc)
	input := testCompleteCheckoutInput()
	input.PointsWon = 5000
	result, err := svc.CompleteCheckout(ctx, session.ID, input)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	o := result.Order
	if _, err := svc.MarkOrderPaid(ctx, o.ID, "pay-1", o.TotalWon); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmOrder(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	attachTestVideo(t, svc, o.ID)
	if _, err := svc.ShipOrder(ctx, o.ID, "m-1", testShipTracking()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelOrder(ctx, o.ID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if len(pay.canceled) != 1 || points.balance != 5000 {
		t.Fatalf("refunds %v balance %d, want card refund and points back", pay.canceled, points.balance)
	}
}

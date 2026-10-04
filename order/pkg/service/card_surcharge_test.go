package service_test

import (
	"errors"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/service"
)

// The card surcharge is taken on what the customer would otherwise pay: goods
// after the promotion, plus delivery. 10,000 − 30% = 7,000, + 3,000 delivery =
// 10,000, + 10% = 11,000.
func TestCompleteCheckout_CardSurchargeOnAmountAfterDiscount(t *testing.T) {
	ctx := t.Context()
	promo := &fakePromotionClient{code: "SUMMER30", discount: 0.30}
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{reservationID: "res-1"}, promo, 0).
		WithProduct(&fakeProduct{defaultKRW: 10000}).
		WithShippingFee(3000).
		WithCardSurcharge(1000)
	session := openSessionWithItem(t, svc)
	if _, err := svc.ApplyCheckoutPromotion(ctx, session.ID, "SUMMER30"); err != nil {
		t.Fatalf("ApplyCheckoutPromotion: %v", err)
	}

	input := testCompleteCheckoutInput()
	input.PaymentMethod = "credit_card"
	result, err := svc.CompleteCheckout(ctx, session.ID, input)
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	o := result.Order
	if o.PaymentMethod != domain.PaymentMethodCreditCard {
		t.Fatalf("payment_method = %q, want credit_card", o.PaymentMethod)
	}
	if o.DiscountWon != 3000 || o.CardSurchargeWon != 1000 || o.TotalWon != 11000 {
		t.Fatalf("discount/surcharge/total = %d/%d/%d, want 3000/1000/11000", o.DiscountWon, o.CardSurchargeWon, o.TotalWon)
	}

	// Payment must be able to charge exactly the total, surcharge included.
	if _, err := svc.MarkOrderPaid(ctx, o.ID, "pay-1", 11000); err != nil {
		t.Fatalf("MarkOrderPaid with the surcharged total: %v", err)
	}
}

// Leaving the method out is a card payment, so it cannot skip the surcharge.
func TestCreateOrder_NoMethodIsCard(t *testing.T) {
	svc := service.New(memory.NewRepository(), &fakeStock{}).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithCardSurcharge(1000)

	o, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o.PaymentMethod != domain.PaymentMethodCreditCard || o.CardSurchargeWon != 25000 || o.TotalWon != 275000 {
		t.Fatalf("method/surcharge/total = %q/%d/%d, want credit_card/25000/275000", o.PaymentMethod, o.CardSurchargeWon, o.TotalWon)
	}
}

// Staff recording an offline payment (bypass) pay no card surcharge.
func TestCreateOrder_BypassHasNoSurcharge(t *testing.T) {
	svc := service.New(memory.NewRepository(), &fakeStock{}).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithCardSurcharge(1000)

	o, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID:    "customer-1",
		Items:         []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
		PaymentMethod: "bypass",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o.PaymentMethod != domain.PaymentMethodBypass || o.CardSurchargeWon != 0 || o.TotalWon != 250000 {
		t.Fatalf("method/surcharge/total = %q/%d/%d, want bypass/0/250000", o.PaymentMethod, o.CardSurchargeWon, o.TotalWon)
	}
}

func TestCompleteCheckout_RejectsUnknownMethod(t *testing.T) {
	svc := service.NewWithCheckout(memory.NewRepository(), &fakeStock{}, nil, 0).
		WithProduct(&fakeProduct{defaultKRW: 10000})
	session := openSessionWithItem(t, svc)

	input := testCompleteCheckoutInput()
	input.PaymentMethod = "bitcoin"
	if _, err := svc.CompleteCheckout(t.Context(), session.ID, input); !errors.Is(err, domain.ErrInvalidPaymentMethod) {
		t.Fatalf("err = %v, want ErrInvalidPaymentMethod", err)
	}
}

func TestWithCardSurcharge_IgnoresNegative(t *testing.T) {
	svc := service.New(memory.NewRepository(), &fakeStock{}).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithCardSurcharge(-1000)

	o, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o.CardSurchargeWon != 0 || o.TotalWon != 250000 {
		t.Fatalf("surcharge/total = %d/%d, want 0/250000", o.CardSurchargeWon, o.TotalWon)
	}
}

package service_test

import (
	"errors"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
)

// An order priced for bypass has no card surcharge in its total, so a card
// checkout on it is refused rather than charging the card without one.
func TestCreatePayment_CardRefusedOnBypassPricedOrder(t *testing.T) {
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalWon: 4200, PaymentMethod: "bypass",
	}}
	svc := service.New(memory.NewRepository(), orders, fakeCheckoutProvider{}, nil)

	_, err := svc.CreatePayment(t.Context(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if !errors.Is(err, ports.ErrMethodMismatch) {
		t.Fatalf("err = %v, want ErrMethodMismatch", err)
	}
}

// A card-priced order charges its total, which already holds the surcharge.
func TestCreatePayment_CardOnCardPricedOrderChargesTotal(t *testing.T) {
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalWon: 11000, PaymentMethod: "credit_card",
	}}
	svc := service.New(memory.NewRepository(), orders, fakeCheckoutProvider{}, nil)

	payment, err := svc.CreatePayment(t.Context(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if payment.AmountWon != 11000 {
		t.Fatalf("amount = %d, want the order total 11000", payment.AmountWon)
	}
}

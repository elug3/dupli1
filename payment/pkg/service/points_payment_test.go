package service_test

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
)

// An order points paid for in full owes ₩0: the shopper's "pay" call records
// a succeeded points payment at once, with no card window, and announces it
// like any other so order moves to paid.
func TestZeroTotalOrderPaysWithPoints(t *testing.T) {
	pub := &recordingPublisher{}
	order := &ports.OrderSummary{ID: "o1", CustomerID: "u-1", Status: "pending", TotalWon: 0, PaymentMethod: "credit_card"}
	svc := service.New(memory.NewRepository(), stubOrderClient{order: order}, fakeCheckoutProvider{}, pub)

	p, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{OrderID: "o1", CustomerID: "u-1", Method: "credit_card"})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if p.Status != domain.StatusSucceeded || p.Method != domain.MethodPoints || p.Provider != domain.ProviderPoints || p.AmountWon != 0 || p.CheckoutURL != "" {
		t.Fatalf("payment = %+v", p)
	}
	if len(pub.events) != 1 || pub.events[0].OrderID != "o1" || pub.events[0].AmountWon != 0 {
		t.Fatalf("events = %+v", pub.events)
	}
	// A retry returns the same payment rather than a second one.
	again, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{OrderID: "o1", CustomerID: "u-1"})
	if err != nil || again.ID != p.ID {
		t.Fatalf("retry = %+v, %v", again, err)
	}
	// A client cannot ask for the points method itself.
	if _, err := domain.NormalizeMethod("points"); err == nil {
		t.Fatal("points accepted as a client method")
	}
}

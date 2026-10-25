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

type fakeMethodGate struct {
	disabled map[string]string // method → message
	err      error
	asked    []string
}

func (g *fakeMethodGate) MethodDisabled(_ context.Context, method string) (bool, string, error) {
	g.asked = append(g.asked, method)
	if g.err != nil {
		return false, "", g.err
	}
	msg, ok := g.disabled[method]
	return ok, msg, nil
}

func TestCreateOrder_RefusesSwitchedOffMethodBeforeReserving(t *testing.T) {
	stock := &fakeStock{}
	gate := &fakeMethodGate{disabled: map[string]string{"credit_card": "카드 결제 점검 중"}}
	svc := service.New(memory.NewRepository(), stock).
		WithProduct(&fakeProduct{defaultKRW: 10000}).
		WithPaymentMethodGate(gate)

	_, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	var disabled *ports.PaymentMethodDisabledError
	if !errors.As(err, &disabled) || !errors.Is(err, ports.ErrPaymentMethodDisabled) {
		t.Fatalf("err = %v, want PaymentMethodDisabledError", err)
	}
	// No method in the input is a card order, and that is what gets checked.
	if disabled.Method != "credit_card" || disabled.Message != "카드 결제 점검 중" {
		t.Fatalf("disabled = %+v", disabled)
	}
	if len(stock.reservedItems) != 0 {
		t.Fatalf("stock reserved %v for a refused order", stock.reservedItems)
	}

	// The other method is untouched.
	o, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID:    "customer-1",
		Items:         []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
		PaymentMethod: "bypass",
	})
	if err != nil || o.PaymentMethod != domain.PaymentMethodBypass {
		t.Fatalf("bypass order = %v, %v", o, err)
	}
}

func TestCreateOrder_GateUnreachableRefusesBeforeReserving(t *testing.T) {
	stock := &fakeStock{}
	gate := &fakeMethodGate{err: ports.ErrPaymentUnavailable}
	svc := service.New(memory.NewRepository(), stock).
		WithProduct(&fakeProduct{defaultKRW: 10000}).
		WithPaymentMethodGate(gate)

	_, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if !errors.Is(err, ports.ErrPaymentUnavailable) {
		t.Fatalf("CreateOrder with payment unreachable: err = %v, want ErrPaymentUnavailable", err)
	}
	if len(stock.reservedItems) != 0 {
		t.Fatalf("stock reserved %v with payment unreachable", stock.reservedItems)
	}

	// Any other failure to ask is refused the same way.
	gate.err = errors.New("boom")
	if _, err := svc.CreateOrder(t.Context(), service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	}); !errors.Is(err, ports.ErrPaymentUnavailable) {
		t.Fatalf("err = %v, want ErrPaymentUnavailable", err)
	}
}

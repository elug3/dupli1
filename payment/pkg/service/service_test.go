package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
)

type stubOrderClient struct {
	order *ports.OrderSummary
}

func (s stubOrderClient) GetOrder(_ context.Context, _, _ string) (*ports.OrderSummary, error) {
	return s.order, nil
}

type recordingPublisher struct {
	events []ports.PaymentSucceededEvent
}

func (p *recordingPublisher) Publish(_ context.Context, subject string, event any) error {
	if subject == ports.PaymentSucceededSubject {
		ev, ok := event.(ports.PaymentSucceededEvent)
		if !ok {
			return fmt.Errorf("unexpected event type %T", event)
		}
		p.events = append(p.events, ev)
	}
	return nil
}

func TestCreatePayment_DevCheckout(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), nil)

	payment, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if payment.Status != domain.StatusRequiresPayment {
		t.Fatalf("status = %s", payment.Status)
	}
	if payment.CheckoutURL == "" {
		t.Fatal("expected checkout URL")
	}
}

func TestCompletePayment_PublishesEvent(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	pub := &recordingPublisher{}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), pub)

	created, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	paid, err := svc.CompletePayment(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("CompletePayment: %v", err)
	}
	if paid.Status != domain.StatusSucceeded {
		t.Fatalf("status = %s", paid.Status)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d", len(pub.events))
	}
	if pub.events[0].OrderID != "ord_1" || pub.events[0].PaymentID != created.ID {
		t.Fatalf("unexpected event: %+v", pub.events[0])
	}
}

func TestCreatePayment_RejectsNonPendingOrder(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "paid", TotalCents: 4200,
	}}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), nil)

	_, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

type failOncePublisher struct {
	failFirst bool
	events    []ports.PaymentSucceededEvent
}

func (p *failOncePublisher) Publish(_ context.Context, subject string, event any) error {
	if subject != ports.PaymentSucceededSubject {
		return nil
	}
	if p.failFirst {
		p.failFirst = false
		return fmt.Errorf("nats unavailable")
	}
	ev, ok := event.(ports.PaymentSucceededEvent)
	if !ok {
		return fmt.Errorf("unexpected event type %T", event)
	}
	p.events = append(p.events, ev)
	return nil
}

func TestCompletePayment_RepublishesAfterPriorPublishFailure(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	pub := &failOncePublisher{failFirst: true}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), pub)

	created, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	if _, err := svc.CompletePayment(context.Background(), created.ID); err == nil {
		t.Fatal("expected first CompletePayment to fail on publish")
	}
	paid, err := repo.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get after failed publish: %v", err)
	}
	if paid.Status != domain.StatusSucceeded {
		t.Fatalf("status after failed publish = %s, want succeeded", paid.Status)
	}

	if _, err := svc.CompletePayment(context.Background(), created.ID); err != nil {
		t.Fatalf("retry CompletePayment: %v", err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events after retry = %d, want 1", len(pub.events))
	}
}

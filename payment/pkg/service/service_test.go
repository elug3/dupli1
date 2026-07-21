package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
	if payment.Method != domain.MethodCreditCard {
		t.Fatalf("method = %s, want %s", payment.Method, domain.MethodCreditCard)
	}
	if payment.CheckoutURL == "" {
		t.Fatal("expected checkout URL")
	}
	if payment.Currency != domain.DefaultCurrency {
		t.Fatalf("currency = %q, want %q", payment.Currency, domain.DefaultCurrency)
	}
	if domain.DefaultCurrency != "krw" {
		t.Fatalf("DefaultCurrency = %q, want krw", domain.DefaultCurrency)
	}
}

func TestCreatePayment_BypassSucceedsAndPublishes(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 70000,
	}}
	pub := &recordingPublisher{}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), pub)

	payment, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID:           "ord_1",
		CustomerID:        "manager_1",
		BearerToken:       "token",
		Method:            domain.MethodBypass,
		Note:              "Cash at showroom",
		CreatedBy:         "manager_1",
		AllowMethodBypass: true,
	})
	if err != nil {
		t.Fatalf("CreatePayment bypass: %v", err)
	}
	if payment.Status != domain.StatusSucceeded {
		t.Fatalf("status = %s, want succeeded", payment.Status)
	}
	if payment.Method != domain.MethodBypass {
		t.Fatalf("method = %s", payment.Method)
	}
	if payment.Provider != domain.ProviderBypass {
		t.Fatalf("provider = %s", payment.Provider)
	}
	if payment.CheckoutURL != "" {
		t.Fatalf("checkout_url should be empty, got %q", payment.CheckoutURL)
	}
	if payment.CreatedBy != "manager_1" || payment.Note != "Cash at showroom" {
		t.Fatalf("audit fields: created_by=%q note=%q", payment.CreatedBy, payment.Note)
	}
	if payment.AmountCents != 70000 {
		t.Fatalf("amount = %d", payment.AmountCents)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d, want 1", len(pub.events))
	}
	if pub.events[0].OrderID != "ord_1" || pub.events[0].AmountCents != 70000 {
		t.Fatalf("unexpected event: %+v", pub.events[0])
	}
}

func TestCreatePayment_BypassForbiddenWithoutPermission(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), nil)

	_, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", Method: domain.MethodBypass,
	})
	if !errors.Is(err, ports.ErrPaymentForbidden) {
		t.Fatalf("err = %v, want ErrPaymentForbidden", err)
	}
}

func TestCreatePayment_BitcoinUnavailable(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), nil)

	_, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", Method: domain.MethodBitcoin,
	})
	if !errors.Is(err, ports.ErrMethodUnavailable) {
		t.Fatalf("err = %v, want ErrMethodUnavailable", err)
	}
}

func TestCreatePayment_UnknownMethod(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), nil)

	_, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", Method: "venmo",
	})
	if !errors.Is(err, domain.ErrInvalidPayment) {
		t.Fatalf("err = %v, want ErrInvalidPayment", err)
	}
}

func TestCreatePayment_BypassSkipsCustomerABAC(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), nil)

	payment, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID:           "ord_1",
		CustomerID:        "manager_other",
		Method:            domain.MethodBypass,
		AllowMethodBypass: true,
		CreatedBy:         "manager_other",
	})
	if err != nil {
		t.Fatalf("bypass for other customer order: %v", err)
	}
	if payment.CustomerID != "cust_1" {
		t.Fatalf("payment customer_id = %s, want cust_1", payment.CustomerID)
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

type failAlwaysPublisher struct {
	calls int
}

func (p *failAlwaysPublisher) Publish(_ context.Context, subject string, event any) error {
	p.calls++
	return fmt.Errorf("nats unavailable")
}

func TestCompletePayment_SoftSucceedsWhenPublishFails(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	pub := &failAlwaysPublisher{}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), pub)

	created, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	paid, err := svc.CompletePayment(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("CompletePayment should soft-succeed: %v", err)
	}
	if paid.Status != domain.StatusSucceeded {
		t.Fatalf("status = %s, want succeeded", paid.Status)
	}
	if pub.calls < 1 {
		t.Fatal("expected publish attempt")
	}
	pending, err := repo.ListPendingOutbox(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPendingOutbox: %v", err)
	}
	if len(pending) != 1 || pending[0].Subject != ports.PaymentSucceededSubject {
		t.Fatalf("pending = %+v, want one payment.succeeded", pending)
	}
}

func TestCompletePayment_DrainOutboxAfterPublishFailure(t *testing.T) {
	repo := memory.NewRepository()
	orders := stubOrderClient{order: &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: 4200,
	}}
	failPub := &failAlwaysPublisher{}
	svc := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), failPub)

	created, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if _, err := svc.CompletePayment(context.Background(), created.ID); err != nil {
		t.Fatalf("CompletePayment: %v", err)
	}

	okPub := &recordingPublisher{}
	svcOK := service.New(repo, orders, checkout.NewDevProvider("http://localhost:8080"), okPub)
	if err := svcOK.DrainOutbox(context.Background()); err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}
	if len(okPub.events) != 1 {
		t.Fatalf("events = %d, want 1", len(okPub.events))
	}
	pending, err := repo.ListPendingOutbox(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPendingOutbox: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d, want 0", len(pending))
	}
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

	// Soft-success: first complete persists succeeded + outbox even when publish fails.
	if _, err := svc.CompletePayment(context.Background(), created.ID); err != nil {
		t.Fatalf("CompletePayment soft-success: %v", err)
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
	if len(pub.events) < 1 {
		t.Fatalf("events after retry = %d, want at least 1", len(pub.events))
	}
}

func TestReconcileSucceededPaymentsRepublishes(t *testing.T) {
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
	if _, err := svc.CompletePayment(context.Background(), created.ID); err != nil {
		t.Fatalf("CompletePayment: %v", err)
	}
	pub.events = nil

	if err := svc.ReconcileSucceededPayments(context.Background(), time.Hour); err != nil {
		t.Fatalf("ReconcileSucceededPayments: %v", err)
	}
	if len(pub.events) != 1 || pub.events[0].PaymentID != created.ID {
		t.Fatalf("reconcile events = %+v", pub.events)
	}
}

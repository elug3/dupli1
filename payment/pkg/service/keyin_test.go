package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
)

type fakeApprover struct {
	calls  int
	last   ports.CardApprovalInput
	result *ports.CardApprovalResult
	err    error
}

func (f *fakeApprover) ApproveCard(_ context.Context, input ports.CardApprovalInput) (*ports.CardApprovalResult, error) {
	f.calls++
	f.last = input
	return f.result, f.err
}

type subjectRecorder struct {
	subjects []string
	rejected []ports.PaymentCallbackRejectedEvent
}

func (r *subjectRecorder) Publish(_ context.Context, subject string, event any) error {
	r.subjects = append(r.subjects, subject)
	if ev, ok := event.(ports.PaymentCallbackRejectedEvent); ok {
		r.rejected = append(r.rejected, ev)
	}
	return nil
}

func (r *subjectRecorder) count(subject string) int {
	n := 0
	for _, s := range r.subjects {
		if s == subject {
			n++
		}
	}
	return n
}

type keyInFixture struct {
	svc      *service.Service
	repo     *memory.Repository
	approver *fakeApprover
	pub      *subjectRecorder
	orders   *mutableOrders
	payment  *domain.Payment
}

type mutableOrders struct{ order ports.OrderSummary }

func (m *mutableOrders) GetOrder(_ context.Context, _, _ string) (*ports.OrderSummary, error) {
	o := m.order
	return &o, nil
}

func newKeyInFixture(t *testing.T) *keyInFixture {
	t.Helper()
	repo := memory.NewRepository()
	orders := &mutableOrders{order: ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalWon: 77000,
		RecipientName: "홍길동", RecipientPhone: "010-1234-5678",
	}}
	nano := checkout.NewNanoProvider(checkout.NanoConfig{
		ShopCode: "240000005", LoginID: "shoptest", APIKey: "k",
		EncKey: "Q2Jv7LkNp5X3M8Yc6rW9T1Eb4F6HdKx6", EncIV: "Nx5Lq7Kv4W8Jp6Mu",
	})
	approver := &fakeApprover{result: &ports.CardApprovalResult{
		Approved: true, ResultCode: "0000", ShopCode: "240000005", ReqPayAmt: "77000", TranNo: "2409030071109",
	}}
	pub := &subjectRecorder{}
	svc := service.New(repo, orders, nano, pub).WithCardApprover(approver, "240000005")
	payment, err := svc.CreatePayment(t.Context(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "cust_1", PayerEmail: "shopper@example.com", BearerToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if payment.CheckoutURL != "" {
		t.Fatalf("key-in payment checkout_url = %q", payment.CheckoutURL)
	}
	return &keyInFixture{svc: svc, repo: repo, approver: approver, pub: pub, orders: orders, payment: payment}
}

func (f *keyInFixture) pay(t *testing.T) (*domain.Payment, error) {
	t.Helper()
	return f.svc.PayByCard(t.Context(), service.PayByCardInput{
		PaymentID: f.payment.ID, CustomerID: "cust_1", BearerToken: "tok",
		Card: domain.CardDetails{Number: "4111111111111111", ExpYY: "30", ExpMM: "07", Password2: "12", HolderID: "900101"},
	})
}

func TestPayByCard_ApprovedMarksPaid(t *testing.T) {
	f := newKeyInFixture(t)
	got, err := f.pay(t)
	if err != nil {
		t.Fatalf("PayByCard: %v", err)
	}
	if got.Status != domain.StatusSucceeded || got.ProviderRef != "2409030071109" {
		t.Fatalf("payment = %+v", got)
	}
	if f.pub.count(ports.PaymentSucceededSubject) != 1 {
		t.Fatalf("payment.succeeded published %d times", f.pub.count(ports.PaymentSucceededSubject))
	}
	in := f.approver.last
	if in.AmountWon != 77000 || in.OrderName != "홍길동" || in.OrderEmail != "shopper@example.com" || in.Card.Installment != "00" {
		t.Fatalf("approval input = %+v", in)
	}
	// A retried submit after success is answered from the record.
	again, err := f.pay(t)
	if err != nil || again.Status != domain.StatusSucceeded || f.approver.calls != 1 {
		t.Fatalf("retry: %v, calls %d", err, f.approver.calls)
	}
}

func TestPayByCard_DeclineLeavesItOpenThenFails(t *testing.T) {
	f := newKeyInFixture(t)
	f.approver.result = &ports.CardApprovalResult{ResultCode: "8035", ResultMsg: "비밀번호 오류"}
	for i := 1; i <= domain.MaxCardAttempts; i++ {
		_, err := f.pay(t)
		var declined *service.CardDeclinedError
		if !errors.As(err, &declined) || declined.Message != "비밀번호 오류" {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if declined.AttemptsRemaining != domain.MaxCardAttempts-i {
			t.Fatalf("attempt %d remaining = %d", i, declined.AttemptsRemaining)
		}
	}
	stored, _ := f.repo.Get(t.Context(), f.payment.ID)
	if stored.Status != domain.StatusFailed {
		t.Fatalf("status = %s", stored.Status)
	}
	if _, err := f.pay(t); !errors.Is(err, domain.ErrCardAttemptsExhausted) {
		t.Fatalf("after exhausting: %v", err)
	}
	if f.approver.calls != domain.MaxCardAttempts {
		t.Fatalf("calls = %d", f.approver.calls)
	}
}

func TestPayByCard_UnconfirmedHoldsAndAlerts(t *testing.T) {
	f := newKeyInFixture(t)
	f.approver.err = ports.ErrCardApprovalUnconfirmed
	f.approver.result = nil
	if _, err := f.pay(t); !errors.Is(err, domain.ErrCardApprovalHeld) {
		t.Fatalf("err = %v", err)
	}
	if len(f.pub.rejected) != 1 || f.pub.rejected[0].Reason != string(service.KeyInRejectUnconfirmed) ||
		f.pub.rejected[0].Source != service.NanoSourceKeyIn || f.pub.rejected[0].ExpectedWon != 77000 {
		t.Fatalf("alerts = %+v", f.pub.rejected)
	}
	// No second charge: the next try never reaches the PG.
	f.approver.err = nil
	if _, err := f.pay(t); !errors.Is(err, domain.ErrCardApprovalHeld) {
		t.Fatalf("second try: %v", err)
	}
	if f.approver.calls != 1 {
		t.Fatalf("calls = %d", f.approver.calls)
	}
	// Starting over from the order hands back the same held payment.
	again, err := f.svc.CreatePayment(t.Context(), service.CreatePaymentInput{OrderID: "ord_1", CustomerID: "cust_1", BearerToken: "tok"})
	if err != nil || again.ID != f.payment.ID || !again.CardApprovalUnconfirmed {
		t.Fatalf("CreatePayment again = %+v, %v", again, err)
	}
}

func TestPayByCard_ApprovalForAnotherAmountIsHeld(t *testing.T) {
	f := newKeyInFixture(t)
	f.approver.result.ReqPayAmt = "1000"
	if _, err := f.pay(t); !errors.Is(err, domain.ErrCardApprovalHeld) {
		t.Fatalf("err = %v", err)
	}
	if len(f.pub.rejected) != 1 || f.pub.rejected[0].Reason != string(service.NanoRejectAmountMismatch) {
		t.Fatalf("alerts = %+v", f.pub.rejected)
	}
	stored, _ := f.repo.Get(t.Context(), f.payment.ID)
	if stored.Status == domain.StatusSucceeded || !stored.CardApprovalUnconfirmed {
		t.Fatalf("payment = %+v", stored)
	}
}

func TestPayByCard_NothingSentWhen(t *testing.T) {
	cases := map[string]func(f *keyInFixture) (service.PayByCardInput, error){
		"card fails checksum": func(f *keyInFixture) (service.PayByCardInput, error) {
			return service.PayByCardInput{PaymentID: f.payment.ID, CustomerID: "cust_1",
				Card: domain.CardDetails{Number: "4111111111111112", ExpYY: "30", ExpMM: "07", Password2: "12", HolderID: "900101"}}, domain.ErrInvalidCard
		},
		"someone else's payment": func(f *keyInFixture) (service.PayByCardInput, error) {
			return service.PayByCardInput{PaymentID: f.payment.ID, CustomerID: "cust_2"}, ports.ErrOrderForbidden
		},
		"order canceled while typing": func(f *keyInFixture) (service.PayByCardInput, error) {
			f.orders.order.Status = "canceled"
			return service.PayByCardInput{PaymentID: f.payment.ID, CustomerID: "cust_1",
				Card: domain.CardDetails{Number: "4111111111111111", ExpYY: "30", ExpMM: "07", Password2: "12", HolderID: "900101"}}, ports.ErrOrderNotPending
		},
		"order total changed": func(f *keyInFixture) (service.PayByCardInput, error) {
			f.orders.order.TotalWon = 1
			return service.PayByCardInput{PaymentID: f.payment.ID, CustomerID: "cust_1",
				Card: domain.CardDetails{Number: "4111111111111111", ExpYY: "30", ExpMM: "07", Password2: "12", HolderID: "900101"}}, domain.ErrInvalidPayment
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newKeyInFixture(t)
			input, want := setup(f)
			if _, err := f.svc.PayByCard(t.Context(), input); !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v", err, want)
			}
			if f.approver.calls != 0 {
				t.Fatal("the PG was called")
			}
			stored, _ := f.repo.Get(t.Context(), f.payment.ID)
			if stored.CardAttempts != 0 {
				t.Fatalf("attempts = %d", stored.CardAttempts)
			}
		})
	}
}

func TestPayByCard_NotSentGivesTheAttemptBack(t *testing.T) {
	f := newKeyInFixture(t)
	f.approver.err = errors.New("encrypt failed")
	f.approver.result = nil
	if _, err := f.pay(t); err == nil || errors.Is(err, domain.ErrCardApprovalHeld) {
		t.Fatalf("err = %v", err)
	}
	stored, _ := f.repo.Get(t.Context(), f.payment.ID)
	if stored.CardAttempts != 0 || stored.CardApprovalStartedAt != nil || stored.Status != domain.StatusRequiresPayment {
		t.Fatalf("payment = %+v", stored)
	}
}

func TestPayByCard_UnavailableWithoutApprover(t *testing.T) {
	svc := service.New(memory.NewRepository(), stubOrderClient{}, fakeCheckoutProvider{}, nil)
	if _, err := svc.PayByCard(t.Context(), service.PayByCardInput{PaymentID: "pay_1"}); !errors.Is(err, ports.ErrMethodUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

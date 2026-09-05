package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

const cancelTestSecret = "test-secret"

// cancelOrderClient serves a pending order of a configurable total, so cancel
// amounts can be exercised against something other than the shared stub's fixed
// 1000 KRW order.
type cancelOrderClient struct {
	totalCents int64
}

func (c cancelOrderClient) GetOrder(_ context.Context, _, _ string) (*ports.OrderSummary, error) {
	return &ports.OrderSummary{
		ID: "ord_1", CustomerID: "cust_1", Status: "pending", TotalCents: c.totalCents,
	}, nil
}

// newCancelFixture wires a mux over a repo holding one succeeded bypass payment
// (bypass keeps the fixture off the PG path, which has its own tests).
func newCancelFixture(t *testing.T, totalCents int64) (*http.ServeMux, *domain.Payment) {
	t.Helper()
	repo := memory.NewRepository()
	svc := service.New(repo, cancelOrderClient{totalCents: totalCents}, fakeCheckoutProvider{}, nil)
	h := handler.New(svc, authjwt.NewHMACValidator(cancelTestSecret))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	payment, err := svc.CreatePayment(t.Context(), service.CreatePaymentInput{
		OrderID: "ord_1", CustomerID: "mgr-1", BearerToken: "token",
		Method: domain.MethodBypass, CreatedBy: "mgr-1", AllowMethodBypass: true,
	})
	if err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	return mux, payment
}

func cancelRequest(t *testing.T, mux *http.ServeMux, paymentID, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = []byte(`{}`)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/"+paymentID+"/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	mux.ServeHTTP(rec, req)
	return rec
}

func TestCancelPayment_RequiresAuth(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	rec := cancelRequest(t, mux, payment.ID, "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}
}

// Refunds are staff-only: there is no ABAC path that lets the paying customer
// cancel their own payment.
func TestCancelPayment_CustomerForbiddenOnOwnPayment(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	rec := cancelRequest(t, mux, payment.ID, makeToken(t, cancelTestSecret, "cust_1", nil), nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelPayment_OtherPaymentPermissionsAreNotEnough(t *testing.T) {
	for _, perm := range []string{permissions.PaymentCreate, permissions.PaymentReadAll, permissions.PaymentBypass} {
		mux, payment := newCancelFixture(t, 70000)
		rec := cancelRequest(t, mux, payment.ID, makeToken(t, cancelTestSecret, "mgr-1", []string{perm}), nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403; body: %s", perm, rec.Code, rec.Body.String())
		}
	}
}

func TestCancelPayment_WithPermissionCancelsFully(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	rec := cancelRequest(t, mux, payment.ID, token, []byte(`{"reason":"ops reject"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got domain.Payment
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", got.Status)
	}
	if got.CanceledAmountCents != 70000 {
		t.Fatalf("canceled = %d, want the full 70000", got.CanceledAmountCents)
	}
	if got.CanceledBy != "mgr-1" || got.CancelReason != "ops reject" {
		t.Fatalf("audit = %q / %q", got.CanceledBy, got.CancelReason)
	}
	if got.CanceledAt == nil {
		t.Fatal("canceled_at must be set in the response")
	}
}

// An empty body is a valid full cancel — ops hitting the endpoint with no
// payload should not get a 400.
func TestCancelPayment_EmptyBodyIsFullCancel(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/"+payment.ID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got domain.Payment
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", got.Status)
	}
}

func TestCancelPayment_PartialAmount(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	rec := cancelRequest(t, mux, payment.ID, token, []byte(`{"amount_cents":20000}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got domain.Payment
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusSucceeded {
		t.Fatalf("status = %q, want succeeded after partial", got.Status)
	}
	if got.CanceledAmountCents != 20000 {
		t.Fatalf("canceled = %d, want 20000", got.CanceledAmountCents)
	}
}

func TestCancelPayment_AmountOverTotalIsBadRequest(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	rec := cancelRequest(t, mux, payment.ID, token, []byte(`{"amount_cents":70001}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelPayment_NegativeAmountIsBadRequest(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	rec := cancelRequest(t, mux, payment.ID, token, []byte(`{"amount_cents":-1}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelPayment_SecondCancelConflicts(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	if rec := cancelRequest(t, mux, payment.ID, token, nil); rec.Code != http.StatusOK {
		t.Fatalf("first cancel status = %d; body: %s", rec.Code, rec.Body.String())
	}
	rec := cancelRequest(t, mux, payment.ID, token, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelPayment_UnknownPaymentIsNotFound(t *testing.T) {
	mux, _ := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	rec := cancelRequest(t, mux, "pay_missing", token, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelPayment_RejectsNonPostMethods(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/payments/"+payment.ID+"/cancel", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", method, rec.Code)
		}
	}
}

// The retry guard has to work end to end, since the Idempotency-Key only
// reaches the service through the handler.
func TestCancelPayment_IdempotencyKeyHeaderIsHonored(t *testing.T) {
	mux, payment := newCancelFixture(t, 70000)
	token := makeToken(t, cancelTestSecret, "mgr-1", []string{permissions.PaymentCancel})

	send := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/"+payment.ID+"/cancel",
			bytes.NewReader([]byte(`{"amount_cents":20000}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", "retry-1")
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := send(); rec.Code != http.StatusOK {
		t.Fatalf("first status = %d; body: %s", rec.Code, rec.Body.String())
	}
	rec := send()
	if rec.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got domain.Payment
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.CanceledAmountCents != 20000 {
		t.Fatalf("canceled = %d, want 20000 — the retry must not refund twice", got.CanceledAmountCents)
	}
}

package httppayment

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/order/pkg/ports"
)

func TestMethodDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/payments/methods" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"methods":[{"method":"credit_card","available":false,"disabled":true,"message":"점검 중"},{"method":"bypass","available":true}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, srv.Client(), nil)

	disabled, msg, err := c.MethodDisabled(t.Context(), "credit_card")
	if err != nil || !disabled || msg != "점검 중" {
		t.Fatalf("credit_card = %v %q %v", disabled, msg, err)
	}
	if disabled, _, err := c.MethodDisabled(t.Context(), "bypass"); err != nil || disabled {
		t.Fatalf("bypass = %v %v", disabled, err)
	}
	if disabled, _, err := c.MethodDisabled(t.Context(), "bitcoin"); err != nil || disabled {
		t.Fatalf("unlisted = %v %v", disabled, err)
	}
}

func TestMethodDisabledNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Card without NANO credentials: no switch engaged, still unusable.
		// Bypass from a payment build that predates "available".
		_, _ = w.Write([]byte(`{"methods":[{"method":"credit_card","available":false,"configured":false,"disabled":false},{"method":"bypass","disabled":false}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, srv.Client(), nil)

	disabled, msg, err := c.MethodDisabled(t.Context(), "credit_card")
	if err != nil || !disabled || msg != "" {
		t.Fatalf("unconfigured credit_card = %v %q %v", disabled, msg, err)
	}
	if disabled, _, err := c.MethodDisabled(t.Context(), "bypass"); err != nil || disabled {
		t.Fatalf("bypass without available = %v %v", disabled, err)
	}
}

func TestMethodDisabledUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	_, _, err := NewClient(srv.URL, srv.Client(), nil).MethodDisabled(t.Context(), "credit_card")
	if !errors.Is(err, ports.ErrPaymentUnavailable) {
		t.Fatalf("err = %v, want ErrPaymentUnavailable", err)
	}
}

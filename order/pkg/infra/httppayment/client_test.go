package httppayment_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/elug3/dupli1/order/pkg/infra/httppayment"
	"github.com/elug3/dupli1/order/pkg/ports"
)

func TestCancelPayment_PostsFullCancel(t *testing.T) {
	var gotPath, gotKey, gotAuth, gotReason string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("Idempotency-Key")
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &body)
		gotReason = body.Reason
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"pay_000026","status":"canceled"}`))
	}))
	defer srv.Close()

	client := httppayment.NewClientWithBearer(srv.URL, srv.Client(), "svc-token")
	if err := client.CancelPayment(t.Context(), "pay_000026", "order-cancel-ord_000036"); err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if gotPath != "/api/v1/payments/pay_000026/cancel" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotKey != "order-cancel-ord_000036" {
		t.Fatalf("idempotency key = %q", gotKey)
	}
	if gotAuth != "Bearer svc-token" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotReason != "order canceled" {
		t.Fatalf("reason = %q", gotReason)
	}
}

func TestCancelPayment_PrefersCallerBearer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := httppayment.NewClientWithBearer(srv.URL, srv.Client(), "svc-token")
	ctx := ports.WithPaymentBearer(t.Context(), "Bearer operator-token")
	if err := client.CancelPayment(ctx, "pay_1", "k"); err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if gotAuth != "Bearer operator-token" {
		t.Fatalf("auth = %q, want the operator token rather than the service account", gotAuth)
	}
}

func TestCancelPayment_RejectedByProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"payment provider rejected the cancel: 취소권한 없음"}`))
	}))
	defer srv.Close()

	client := httppayment.NewClientWithBearer(srv.URL, srv.Client(), "tok")
	err := client.CancelPayment(t.Context(), "pay_1", "k")
	if !errors.Is(err, ports.ErrPaymentRefundRejected) {
		t.Fatalf("err = %v, want ErrPaymentRefundRejected", err)
	}
}

func TestCancelPayment_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden: insufficient permission"}`))
	}))
	defer srv.Close()

	client := httppayment.NewClientWithBearer(srv.URL, srv.Client(), "tok")
	err := client.CancelPayment(t.Context(), "pay_1", "k")
	if !errors.Is(err, ports.ErrPaymentForbidden) {
		t.Fatalf("err = %v, want ErrPaymentForbidden", err)
	}
}

func TestCancelPayment_ConflictAlreadyCanceledIsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":"pay_1","status":"canceled"}`))
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"payment is not cancelable"}`))
	}))
	defer srv.Close()

	client := httppayment.NewClientWithBearer(srv.URL, srv.Client(), "tok")
	if err := client.CancelPayment(t.Context(), "pay_1", "k"); err != nil {
		t.Fatalf("already-canceled conflict: %v", err)
	}
}

func TestCancelPayment_RetriesOnceOnUnauthorized(t *testing.T) {
	var hits atomic.Int32
	src := &flakyTokenSource{tokens: []string{"stale", "fresh"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fresh" {
			t.Errorf("retry auth = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := httppayment.NewClient(srv.URL, srv.Client(), src)
	if err := client.CancelPayment(t.Context(), "pay_1", "k"); err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", hits.Load())
	}
}

type flakyTokenSource struct {
	tokens []string
	idx    atomic.Int32
}

func (f *flakyTokenSource) Token(context.Context) (string, error) {
	i := int(f.idx.Load())
	if i >= len(f.tokens) {
		return f.tokens[len(f.tokens)-1], nil
	}
	return f.tokens[i], nil
}

func (f *flakyTokenSource) Invalidate() { f.idx.Add(1) }

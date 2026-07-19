package httpstock_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
	"github.com/elug3/dupli1/order/pkg/infra/httpstock"
	"github.com/elug3/dupli1/order/pkg/ports"
)

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

func (f *flakyTokenSource) Invalidate() {
	f.idx.Add(1)
}

func TestClient_RetriesOnceOnUnauthorized(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		auth := r.Header.Get("Authorization")
		if n == 1 {
			if auth != "Bearer stale" {
				t.Errorf("first auth = %q, want Bearer stale", auth)
			}
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		if auth != "Bearer fresh" {
			t.Errorf("retry auth = %q, want Bearer fresh", auth)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"reservation_id": "res-1"})
	}))
	defer srv.Close()

	src := &flakyTokenSource{tokens: []string{"stale", "fresh"}}
	client := httpstock.NewClient(srv.URL, srv.Client(), src)

	id, err := client.Reserve(context.Background(), "ord-1", []ports.StockItem{
		{SKU: "SKU-1", Quantity: 1},
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if id != "res-1" {
		t.Fatalf("id = %q", id)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", hits.Load())
	}
}

func TestClient_UnauthorizedErrorIsClear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer srv.Close()

	client := httpstock.NewClientWithBearer(srv.URL, srv.Client(), "bad")
	err := client.CommitReservation(context.Background(), "res-1")
	if !errors.Is(err, httpstock.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if got := err.Error(); got != "product stock request failed: unauthorized" {
		t.Fatalf("err text = %q", got)
	}
}

func TestClient_StaticBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixed" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	client := httpstock.NewClientWithBearer(srv.URL, srv.Client(), "fixed")
	if err := client.CommitReservation(context.Background(), "res-1"); err != nil {
		t.Fatal(err)
	}
}

func TestClient_UsesTokenSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer from-source" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	client := httpstock.NewClient(srv.URL, srv.Client(), httpauth.StaticToken("from-source"))
	if err := client.ReleaseReservation(context.Background(), "res-1"); err != nil {
		t.Fatal(err)
	}
}

func TestClient_NoTokenSource(t *testing.T) {
	client := httpstock.NewClient("http://example.invalid", nil, nil)
	_, err := client.Reserve(context.Background(), "ord-1", []ports.StockItem{{SKU: "S", Quantity: 1}})
	if !errors.Is(err, httpstock.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

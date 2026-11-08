package gateway

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/product/pkg/ports"
)

func TestOrderReader_ForwardsTheShoppersToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer shopper-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/orders/ORD-1":
			_, _ = w.Write([]byte(`{"id":"ORD-1","customer_id":"c1","status":"fulfilled","items":[{"sku_id":"S1","sku":"DUP_A"},{"sku":"DUP_B"}]}`))
		case "/api/v1/orders/ORD-THEIRS":
			w.WriteHeader(http.StatusForbidden)
		case "/api/v1/orders/ORD-DOWN":
			w.WriteHeader(http.StatusBadGateway)
		case "/api/v1/profile/me/profile":
			_, _ = w.Write([]byte(`{"user_id":"c1","display_name":"김민지","addresses":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	orders := NewOrderReader(srv.URL+"/", nil)
	o, err := orders.Order(t.Context(), "shopper-token", "ORD-1")
	if err != nil || o.CustomerID != "c1" || o.Status != "fulfilled" || len(o.Lines) != 2 || o.Lines[1].SKU != "DUP_B" {
		t.Fatalf("order: %+v %v", o, err)
	}
	for _, id := range []string{"ORD-THEIRS", "ORD-MISSING"} {
		if _, err := orders.Order(t.Context(), "shopper-token", id); !errors.Is(err, ports.ErrOrderNotReadable) {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if _, err := orders.Order(t.Context(), "shopper-token", "ORD-DOWN"); !errors.Is(err, ports.ErrReferenceUnavailable) {
		t.Fatalf("order down: %v", err)
	}
	if _, err := orders.Order(t.Context(), "", "ORD-1"); !errors.Is(err, ports.ErrOrderNotReadable) {
		t.Fatalf("no token: %v", err)
	}

	name, err := NewProfileReader(srv.URL, nil).DisplayName(t.Context(), "shopper-token")
	if err != nil || name != "김민지" {
		t.Fatalf("profile: %q %v", name, err)
	}
}

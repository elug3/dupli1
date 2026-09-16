package httppromotion_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
	"github.com/elug3/dupli1/order/pkg/infra/httppromotion"
	"github.com/elug3/dupli1/order/pkg/ports"
)

func TestClientEvaluateDoesNotSendAuth(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products/promotions/evaluate" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":           true,
			"discount_won": 5000,
		})
	}))
	t.Cleanup(srv.Close)

	client := httppromotion.NewClient(srv.URL, srv.Client(), httpauth.StaticToken("should-not-appear"))
	got, err := client.Evaluate(t.Context(), "WELCOME", ports.PromotionContext{
		CustomerID:     "cust-1",
		ShippingFeeWon: 3000,
		Lines: []ports.PromotionLine{
			{SkuID: "sku-1", SKU: "sku-1", Quantity: 1, UnitPriceWon: 50000},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if auth != "" {
		t.Fatalf("evaluate sent Authorization %q, want none", auth)
	}
	if !got.OK || got.DiscountWon != 5000 || got.Code != "WELCOME" {
		t.Fatalf("result = %+v, want ok/discount/code pinned", got)
	}
}

func TestClientEvaluateNotFoundMapsToInvalidCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := httppromotion.NewClient(srv.URL, srv.Client(), nil)
	got, err := client.Evaluate(t.Context(), "NOPE", ports.PromotionContext{CustomerID: "cust-1"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got.OK || got.Reason != "invalid_code" || got.Code != "NOPE" {
		t.Fatalf("result = %+v, want invalid_code refusal", got)
	}
}

func TestClientReserveUnwrapsNestedResult(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products/promotions/reserve" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"ok":           true,
				"discount_won":   5000,
				"reason":         "",
				"sub_reason":     "",
			},
			"redemption": map[string]any{"order_id": "ord-1", "code": "WELCOME"},
		})
	}))
	t.Cleanup(srv.Close)

	client := httppromotion.NewClient(srv.URL, srv.Client(), httpauth.StaticToken("svc-token"))
	got, err := client.Reserve(t.Context(), "WELCOME", "ord-1", ports.PromotionContext{
		CustomerID: "cust-1",
		Lines:      []ports.PromotionLine{{SkuID: "sku-1", Quantity: 1, UnitPriceWon: 50000}},
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if auth != "Bearer svc-token" {
		t.Fatalf("auth = %q, want Bearer svc-token", auth)
	}
	if !got.OK || got.DiscountWon != 5000 || got.Code != "WELCOME" {
		t.Fatalf("result = %+v, want unwrapped reserve verdict", got)
	}
}

func TestClientConsumeAndReleaseSendAuth(t *testing.T) {
	var paths []string
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		auths = append(auths, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	client := httppromotion.NewClient(srv.URL, srv.Client(), httpauth.StaticToken("ledger-token"))
	if err := client.Consume(t.Context(), "ord-1"); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if err := client.Release(t.Context(), "ord-2"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want consume then release", paths)
	}
	if paths[0] != "/api/v1/products/promotions/consume" || paths[1] != "/api/v1/products/promotions/release" {
		t.Fatalf("unexpected paths: %v", paths)
	}
	for i, want := range []string{"Bearer ledger-token", "Bearer ledger-token"} {
		if auths[i] != want {
			t.Fatalf("call %d auth = %q, want %q", i, auths[i], want)
		}
	}
}

func TestClientUpstreamErrorIncludesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "order_id is required"})
	}))
	t.Cleanup(srv.Close)

	client := httppromotion.NewClient(srv.URL, srv.Client(), httpauth.StaticToken("token"))
	err := client.Consume(t.Context(), "")
	if err == nil {
		t.Fatal("expected error for bad upstream response")
	}
	if !strings.Contains(err.Error(), "order_id is required") {
		t.Fatalf("err = %q, want upstream error text", err.Error())
	}
}

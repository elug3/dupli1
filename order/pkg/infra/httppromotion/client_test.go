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
				"discount_won": 5000,
				"reason":       "",
				"sub_reason":   "",
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

// The evaluator resolves a line's category, brand, parent and sale state from
// product's own catalog, keyed on the identifiers below. Dropping either
// identifier would leave those conditions unresolvable, so the shape is pinned
// here rather than left to whoever next edits the struct.
func TestClientSendsLineIdentifiersForCatalogLookup(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "discount_won": 0})
	}))
	t.Cleanup(srv.Close)

	client := httppromotion.NewClient(srv.URL, srv.Client(), httpauth.StaticToken("t"))
	if _, err := client.Evaluate(t.Context(), "CODE", ports.PromotionContext{
		CustomerID:     "cust-1",
		ShippingFeeWon: 3000,
		Lines: []ports.PromotionLine{
			{SkuID: "01J8SKU", SKU: "PRA_GALLERIA_BLK_M", Quantity: 2, UnitPriceWon: 50000},
		},
	}); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	lines, ok := body["lines"].([]any)
	if !ok || len(lines) != 1 {
		t.Fatalf("lines = %#v, want one line", body["lines"])
	}
	line, ok := lines[0].(map[string]any)
	if !ok {
		t.Fatalf("line = %#v", lines[0])
	}
	for key, want := range map[string]any{
		"sku_id":         "01J8SKU",
		"sku":            "PRA_GALLERIA_BLK_M",
		"quantity":       float64(2),
		"unit_price_won": float64(50000),
	} {
		if line[key] != want {
			t.Fatalf("line[%q] = %#v, want %#v", key, line[key], want)
		}
	}
	// Catalog attributes are product's to resolve, not order's to assert.
	for _, key := range []string{"category", "brand_code", "product_id", "on_sale"} {
		if _, present := line[key]; present {
			t.Fatalf("line carries %q; the evaluator reads that from the catalog", key)
		}
	}
}

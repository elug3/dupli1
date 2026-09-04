package httpproduct_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/cart/pkg/infra/httpproduct"
	"github.com/elug3/dupli1/cart/pkg/ports"
)

// The full HTTP-fetching behavior (decoding, KRW conversion, trailing
// slash, etc.) is covered by shared/pkg/productclient's own tests. These
// tests only cover what's specific to this wrapper: mapping the shared
// superset Variant into cart's own ports.VariantInfo (Color, not
// ProductName), and translating the shared sentinel error into cart's own.
func TestGetVariantMapsColor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skuId":     "01HX",
			"sku":       "BOT-001-GRN",
			"productId": "BOT-001",
			"color":     "Green",
			"price":     2890000.0,
			"imageUrls": []string{},
		})
	}))
	defer srv.Close()

	client := httpproduct.NewClient(srv.URL, srv.Client())
	info, err := client.GetVariant(t.Context(), "BOT-001-GRN")
	if err != nil {
		t.Fatal(err)
	}
	if info.Color != "Green" {
		t.Fatalf("Color = %q, want Green", info.Color)
	}
	if info.UnitPriceCents != 2890000 {
		t.Fatalf("UnitPriceCents=%d, want 2890000 (KRW won, not ×100)", info.UnitPriceCents)
	}
}

func TestGetVariantBySkuIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := httpproduct.NewClient(srv.URL, srv.Client())
	_, err := client.GetVariantBySkuID(t.Context(), "missing")
	if !errors.Is(err, ports.ErrVariantNotFound) {
		t.Fatalf("want ports.ErrVariantNotFound, got %v", err)
	}
}

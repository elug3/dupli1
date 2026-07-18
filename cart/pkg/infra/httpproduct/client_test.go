package httpproduct_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/cart/pkg/infra/httpproduct"
)

func TestGetVariantUsesWholeWon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skuId":     "01HX",
			"sku":       "BOT-001-GRN",
			"productId": "BOT-001",
			"color":     "Green",
			"price":     2890000.0,
			"status":    "active",
			"imageUrls": []string{},
		})
	}))
	defer srv.Close()

	client := httpproduct.NewClient(srv.URL, srv.Client())
	info, err := client.GetVariant(context.Background(), "BOT-001-GRN")
	if err != nil {
		t.Fatal(err)
	}
	if info.UnitPriceCents != 2890000 {
		t.Fatalf("UnitPriceCents=%d, want 2890000 (KRW won, not ×100)", info.UnitPriceCents)
	}
}

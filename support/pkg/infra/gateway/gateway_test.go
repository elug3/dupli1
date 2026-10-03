package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/support/pkg/ports"
)

func TestOrderReaderForwardsBearerAndMapsForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer shopper-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/orders/mine":
			_, _ = w.Write([]byte(`{"id":"mine","status":"paid","total_won":120000,"created_at":"2026-10-01T00:00:00Z",
				"items":[{"product_name":"Galleria","sku":"PRADA_GAL_BLK_M","quantity":1},{"sku":"X","quantity":2}]}`))
		case "/api/v1/orders/theirs":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reader := NewOrderReader(srv.URL, nil)
	ref, err := reader.Order(context.Background(), "shopper-token", "mine")
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	if ref.Status != "paid" || ref.TotalWon != 120000 || ref.ItemCount != 2 || ref.FirstItemName != "Galleria" {
		t.Fatalf("ref = %+v", ref)
	}

	for _, id := range []string{"theirs", "missing"} {
		if _, err := reader.Order(context.Background(), "shopper-token", id); !errors.Is(err, ports.ErrReferenceNotFound) {
			t.Fatalf("Order(%s) err = %v, want ErrReferenceNotFound", id, err)
		}
	}
}

func TestProductReaderUsesVariantEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products/variants/by-sku-id/01SKU" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"skuId":"01SKU","sku":"prada_gal_blk_m","productId":"01P","color":"Black",
			"price":2350000,"productName":"Galleria","imageUrls":["https://img/1.jpg"]}`))
	}))
	defer srv.Close()

	reader := NewProductReader(srv.URL, nil)
	ref, err := reader.Variant(context.Background(), "01SKU")
	if err != nil {
		t.Fatalf("Variant: %v", err)
	}
	if ref.ProductID != "01P" || ref.SKU != "PRADA_GAL_BLK_M" || ref.PriceWon != 2350000 || ref.ImageURL != "https://img/1.jpg" {
		t.Fatalf("ref = %+v", ref)
	}
	if _, err := reader.Variant(context.Background(), "nope"); !errors.Is(err, ports.ErrReferenceNotFound) {
		t.Fatalf("missing variant err = %v", err)
	}
}

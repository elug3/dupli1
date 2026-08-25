package httpproduct

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/order/pkg/ports"
)

func TestClientGetVariantMapsProductNameAndImageURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products/variants/by-sku/BAG-001" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"skuId":"sku-bag-1",
			"sku":"BAG-001",
			"productId":"prod-1",
			"price":250000,
			"productName":"Prada Galleria",
			"imageUrls":["https://cdn.example/a.jpg","https://cdn.example/b.jpg"]
		}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, srv.Client())
	info, err := client.GetVariant(context.Background(), "BAG-001")
	if err != nil {
		t.Fatalf("GetVariant: %v", err)
	}
	if info.ProductName != "Prada Galleria" {
		t.Fatalf("ProductName = %q, want Prada Galleria", info.ProductName)
	}
	if info.ImageURL != "https://cdn.example/a.jpg" {
		t.Fatalf("ImageURL = %q, want first image URL", info.ImageURL)
	}
	if info.UnitPriceCents != 250000 {
		t.Fatalf("UnitPriceCents = %d, want 250000", info.UnitPriceCents)
	}
}

func TestClientGetVariantBySkuIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, srv.Client())
	_, err := client.GetVariantBySkuID(context.Background(), "missing")
	if !errors.Is(err, ports.ErrVariantNotFound) {
		t.Fatalf("want ErrVariantNotFound, got %v", err)
	}
}

package productclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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

	client := NewClient(srv.URL, srv.Client())
	v, err := client.GetVariant(context.Background(), "BOT-001-GRN")
	if err != nil {
		t.Fatal(err)
	}
	if v.UnitPriceCents != 2890000 {
		t.Fatalf("UnitPriceCents=%d, want 2890000 (KRW won, not ×100)", v.UnitPriceCents)
	}
	if v.Color != "Green" {
		t.Fatalf("Color = %q, want Green", v.Color)
	}
}

func TestGetVariantMapsProductNameAndImageURL(t *testing.T) {
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
	v, err := client.GetVariant(context.Background(), "BAG-001")
	if err != nil {
		t.Fatalf("GetVariant: %v", err)
	}
	if v.ProductName != "Prada Galleria" {
		t.Fatalf("ProductName = %q, want Prada Galleria", v.ProductName)
	}
	if v.ImageURL != "https://cdn.example/a.jpg" {
		t.Fatalf("ImageURL = %q, want first image URL", v.ImageURL)
	}
	if v.UnitPriceCents != 250000 {
		t.Fatalf("UnitPriceCents = %d, want 250000", v.UnitPriceCents)
	}
}

func TestGetVariantBySkuIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, srv.Client())
	_, err := client.GetVariantBySkuID(context.Background(), "missing")
	if !errors.Is(err, ErrVariantNotFound) {
		t.Fatalf("want ErrVariantNotFound, got %v", err)
	}
}

func TestGetVariant_ServerErrorIsWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, srv.Client())
	_, err := client.GetVariant(context.Background(), "whatever")
	if err == nil || errors.Is(err, ErrVariantNotFound) {
		t.Fatalf("want a non-404 error, got %v", err)
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"sku": "X"})
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL+"/", srv.Client())
	if _, err := client.GetVariant(context.Background(), "X"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/products/variants/by-sku/X" {
		t.Fatalf("path = %q, trailing slash in baseURL was not trimmed", gotPath)
	}
}

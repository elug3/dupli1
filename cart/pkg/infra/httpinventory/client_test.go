package httpinventory_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/cart/pkg/infra/httpinventory"
)

func TestGetAvailableQty_SubtractsReserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products/inventory/items/BOT-001-GRN" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"quantity": 10, "reserved": 3})
	}))
	defer srv.Close()

	client := httpinventory.NewClient(srv.URL, srv.Client())
	qty, err := client.GetAvailableQty(t.Context(), "BOT-001-GRN")
	if err != nil {
		t.Fatal(err)
	}
	if qty != 7 {
		t.Fatalf("available = %d, want 7", qty)
	}
}

func TestGetAvailableQty_FloorsNegativeAtZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int{"quantity": 2, "reserved": 5})
	}))
	defer srv.Close()

	client := httpinventory.NewClient(srv.URL, srv.Client())
	qty, err := client.GetAvailableQty(t.Context(), "BOT-001-GRN")
	if err != nil {
		t.Fatal(err)
	}
	if qty != 0 {
		t.Fatalf("available = %d, want 0 when reserved exceeds quantity", qty)
	}
}

func TestGetAvailableQty_NotFoundMeansOutOfStock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := httpinventory.NewClient(srv.URL, srv.Client())
	qty, err := client.GetAvailableQty(t.Context(), "MISSING-SKU")
	if err != nil {
		t.Fatal(err)
	}
	if qty != 0 {
		t.Fatalf("404 should map to 0 available (always-tracked OOS), got %d", qty)
	}
}

func TestGetAvailableQtyBySkuID_UsesBySkuIdPath(t *testing.T) {
	const skuID = "01JABC123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1/products/inventory/items/by-sku-id/" + skuID
		if r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"quantity": 4, "reserved": 1})
	}))
	defer srv.Close()

	client := httpinventory.NewClient(srv.URL, srv.Client())
	qty, err := client.GetAvailableQtyBySkuID(t.Context(), skuID)
	if err != nil {
		t.Fatal(err)
	}
	if qty != 3 {
		t.Fatalf("available = %d, want 3", qty)
	}
}

func TestGetAvailableQty_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	client := httpinventory.NewClient(srv.URL, srv.Client())
	_, err := client.GetAvailableQty(t.Context(), "BOT-001-GRN")
	if err == nil {
		t.Fatal("want error on non-2xx upstream")
	}
}

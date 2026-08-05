package httporder_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/infra/httporder"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

func TestGetOrderParsesFulfillmentFields(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/orders/ord-42" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":              "ord-42",
			"customer_id":     "cust-1",
			"status":          "pending",
			"total_cents":     7000,
			"recipient_name":  "윤라희",
			"recipient_phone": "01041125167",
			"shipping_address": map[string]string{
				"postal_code":   "06194",
				"address_line1": "테헤란로 78길 14-12",
				"address_line2": "9층",
				"city":          "강남구",
				"province":      "서울특별시",
			},
		})
	}))
	defer srv.Close()

	client := httporder.NewClient(srv.URL, srv.Client())
	got, err := client.GetOrder(context.Background(), "access-token", "ord-42")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer access-token" {
		t.Fatalf("Authorization = %q, want Bearer access-token", gotAuth)
	}
	if got.RecipientName != "윤라희" || got.RecipientPhone != "01041125167" {
		t.Fatalf("recipient = %q/%q", got.RecipientName, got.RecipientPhone)
	}
	if got.ShippingAddress.PostalCode != "06194" || got.ShippingAddress.AddressLine2 != "9층" {
		t.Fatalf("shipping address = %+v", got.ShippingAddress)
	}
	if got.TotalCents != 7000 || got.Status != "pending" {
		t.Fatalf("order summary = %+v", got)
	}
}

func TestGetOrderNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := httporder.NewClient(srv.URL, srv.Client())
	_, err := client.GetOrder(context.Background(), "token", "missing")
	if err != ports.ErrOrderNotFound {
		t.Fatalf("error = %v, want ErrOrderNotFound", err)
	}
}

func TestGetOrderForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := httporder.NewClient(srv.URL, srv.Client())
	_, err := client.GetOrder(context.Background(), "token", "ord-1")
	if err != ports.ErrOrderForbidden {
		t.Fatalf("error = %v, want ErrOrderForbidden", err)
	}
}

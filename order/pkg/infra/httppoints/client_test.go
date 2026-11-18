package httppoints

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/order/pkg/ports"
)

func TestClientMapsPaymentReasons(t *testing.T) {
	var gotPath, gotBody string
	status, reason := http.StatusOK, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		b, _ := json.Marshal(m)
		gotBody = string(b)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "x", "reason": reason})
	}))
	defer srv.Close()
	c := NewClient(srv.URL, srv.Client(), nil)
	ctx := t.Context()

	if err := c.Reserve(ctx, "o1", "u1", 500); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/payments/points/reserve" || gotBody != `{"customer_id":"u1","order_id":"o1","points":500}` {
		t.Fatalf("reserve sent %s %s", gotPath, gotBody)
	}

	status, reason = http.StatusConflict, "insufficient_points"
	if err := c.Reserve(ctx, "o1", "u1", 500); !errors.Is(err, ports.ErrInsufficientPoints) {
		t.Fatalf("err = %v", err)
	}
	status, reason = http.StatusNotFound, "points_hold_not_found"
	if err := c.Release(ctx, "o1"); err != nil {
		t.Fatalf("release of no hold: %v", err)
	}
	if err := c.Consume(ctx, "o1"); !errors.Is(err, ports.ErrPointsUnavailable) {
		t.Fatalf("consume of no hold: %v", err)
	}
	status, reason = http.StatusBadGateway, ""
	if err := c.Consume(ctx, "o1"); !errors.Is(err, ports.ErrPointsUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

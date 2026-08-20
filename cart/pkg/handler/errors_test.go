package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/cart/pkg/domain"
	"github.com/elug3/dupli1/cart/pkg/ports"
	"github.com/elug3/dupli1/cart/pkg/service"
)

func TestRespondServiceErrorSanitizesInternal(t *testing.T) {
	rec := httptest.NewRecorder()
	respondServiceError(rec, errors.New(`pq: connection refused`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	msg, _ := body["error"].(string)
	if msg != "internal error" {
		t.Fatalf("error = %q, want internal error", msg)
	}
	if strings.Contains(msg, "pq:") {
		t.Fatalf("leaked internal detail: %q", msg)
	}
}

func TestRespondServiceErrorKeepsClientErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{ports.ErrVariantNotFound, http.StatusNotFound},
		{domain.ErrInvalidCart, http.StatusBadRequest},
		{ports.ErrProductUnavailable, http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		respondServiceError(rec, tc.err)
		if rec.Code != tc.code {
			t.Fatalf("%v: status = %d, want %d", tc.err, rec.Code, tc.code)
		}
	}
}

func TestRespondServiceErrorUnavailableItems(t *testing.T) {
	rec := httptest.NewRecorder()
	respondServiceError(rec, &service.UnavailableVariantsError{
		Items: []domain.UnavailableItem{
			{SkuID: "A", SKU: "SKU-A", Reason: domain.ReasonVariantNotFound},
			{SkuID: "B", Reason: domain.ReasonVariantNotFound},
		},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "variant not found" {
		t.Fatalf("error = %v", body["error"])
	}
	items, ok := body["unavailable_items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("unavailable_items = %#v", body["unavailable_items"])
	}
}

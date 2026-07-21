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

package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
)

func TestRespondServiceErrorSanitizesInternal(t *testing.T) {
	rec := httptest.NewRecorder()
	respondServiceError(rec, errors.New(`pq: relation "orders" does not exist`))
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
	if strings.Contains(msg, "pq:") || strings.Contains(msg, "relation") {
		t.Fatalf("leaked internal detail: %q", msg)
	}
}

func TestRespondServiceErrorKeepsClientErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{ports.ErrNotFound, http.StatusNotFound},
		{domain.ErrInvalidOrder, http.StatusBadRequest},
		{domain.ErrSessionNotOpen, http.StatusBadRequest},
		{ports.ErrProductUnavailable, http.StatusBadGateway},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		respondServiceError(rec, tc.err)
		if rec.Code != tc.code {
			t.Fatalf("%v: status = %d, want %d", tc.err, rec.Code, tc.code)
		}
	}
}

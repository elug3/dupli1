package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

func TestRespondServiceErrorMapping(t *testing.T) {
	h := &Handler{}

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "not found sentinel",
			err:        fmt.Errorf("product BOT-001: %w", ports.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantBody:   "not found",
		},
		{
			name:       "master not found",
			err:        fmt.Errorf("%w: brand XX", domain.ErrMasterNotFound),
			wantStatus: http.StatusNotFound,
			wantBody:   "master data not found",
		},
		{
			name:       "conflict",
			err:        ports.Conflict("coupon already exists"),
			wantStatus: http.StatusConflict,
			wantBody:   "coupon already exists",
		},
		{
			name:       "invalid",
			err:        ports.Invalid("name is required"),
			wantStatus: http.StatusBadRequest,
			wantBody:   "name is required",
		},
		{
			name:       "missing sku codes",
			err:        domain.ErrMissingSKUCodes,
			wantStatus: http.StatusBadRequest,
			wantBody:   "required sku master codes missing",
		},
		{
			name:       "raw database error sanitized",
			err:        errors.New(`ERROR: column "bogus" does not exist (SQLSTATE 42703)`),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.respondServiceError(rec, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: want %d, got %d (%s)", tc.wantStatus, rec.Code, rec.Body.String())
			}
			var body ErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body.Error, tc.wantBody) {
				t.Fatalf("body: want containing %q, got %q", tc.wantBody, body.Error)
			}
			if tc.wantStatus == http.StatusInternalServerError && strings.Contains(body.Error, "SQLSTATE") {
				t.Fatalf("leaked SQL details: %q", body.Error)
			}
		})
	}
}

package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/profile/pkg/domain"
	"github.com/elug3/dupli1/profile/pkg/handler"
	"github.com/elug3/dupli1/profile/pkg/infra/memory"
	"github.com/elug3/dupli1/profile/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "handler-test-secret"

func makeToken(t *testing.T, userID string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":  userID,
		"type": "access",
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return signed
}

func newTestHandler(t *testing.T) *handler.Handler {
	t.Helper()
	repo := memory.NewProfileRepository()
	svc := service.New(repo)
	validator := authjwt.NewHMACValidator(testSecret)
	return handler.New(svc, validator)
}

func newMux(h *handler.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func doJSON(t *testing.T, mux *http.ServeMux, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSettingsAndHealthDoNotRequireAuth(t *testing.T) {
	mux := newMux(newTestHandler(t))
	for _, path := range []string{"/settings", "/api/v1/profile/settings", "/health", "/api/v1/profile/health"} {
		rec := doJSON(t, mux, http.MethodGet, path, "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
	}
}

func TestGetProfileRequiresAuth(t *testing.T) {
	mux := newMux(newTestHandler(t))
	rec := doJSON(t, mux, http.MethodGet, "/api/v1/profile/me/profile", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestProfileAndAddresses_CanonicalRoutes(t *testing.T) {
	mux := newMux(newTestHandler(t))
	token := makeToken(t, "user-1")

	rec := doJSON(t, mux, http.MethodGet, "/api/v1/profile/me/profile", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET profile: %d %s", rec.Code, rec.Body.String())
	}
	var view domain.ProfileView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Addresses) != 0 {
		t.Fatalf("expected no addresses: %+v", view)
	}

	rec = doJSON(t, mux, http.MethodPatch, "/api/v1/profile/me/profile", token, map[string]string{
		"display_name": "윤라희",
		"phone":        "010-4112-5167",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH profile: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.DisplayName != "윤라희" || view.Phone != "01041125167" {
		t.Fatalf("profile: %+v", view)
	}

	rec = doJSON(t, mux, http.MethodPost, "/api/v1/profile/me/addresses", token, map[string]string{
		"recipient_name":  "윤라희",
		"recipient_phone": "01041125167",
		"postal_code":     "06194",
		"address_line1":   "테헤란로 78길 14-12",
		"city":            "강남구",
		"province":        "서울특별시",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST address: %d %s", rec.Code, rec.Body.String())
	}
	var addr domain.Address
	if err := json.Unmarshal(rec.Body.Bytes(), &addr); err != nil {
		t.Fatal(err)
	}
	if !addr.IsDefault || addr.ID == "" {
		t.Fatalf("address: %+v", addr)
	}

	rec = doJSON(t, mux, http.MethodGet, "/api/v1/profile/me/addresses/"+addr.ID, token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET address: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodPatch, "/api/v1/profile/me/addresses/"+addr.ID, token, map[string]string{
		"recipient_name": "Updated Recipient",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH address: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodPost, "/api/v1/profile/me/addresses", token, map[string]string{
		"recipient_name":  "Second",
		"recipient_phone": "01022223333",
		"postal_code":     "06194",
		"address_line1":   "Other street",
		"city":            "강남구",
		"province":        "서울특별시",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST second address: %d %s", rec.Code, rec.Body.String())
	}
	var second domain.Address
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}

	rec = doJSON(t, mux, http.MethodPost, "/api/v1/profile/me/addresses/"+second.ID+"/default", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST default: %d %s", rec.Code, rec.Body.String())
	}
	var defaulted domain.Address
	if err := json.Unmarshal(rec.Body.Bytes(), &defaulted); err != nil {
		t.Fatal(err)
	}
	if !defaulted.IsDefault {
		t.Fatal("second address should be default")
	}

	rec = doJSON(t, mux, http.MethodDelete, "/api/v1/profile/me/addresses/"+addr.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE address: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, mux, http.MethodGet, "/api/v1/profile/me/addresses/"+addr.ID, token, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted address GET: want 404, got %d", rec.Code)
	}
}

func TestProfileAndAddresses_LegacyAuthAliasRoutes(t *testing.T) {
	mux := newMux(newTestHandler(t))
	token := makeToken(t, "user-legacy")

	rec := doJSON(t, mux, http.MethodPatch, "/api/v1/auth/me/profile", token, map[string]string{
		"display_name": "Legacy User",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy PATCH profile: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodPost, "/api/v1/auth/me/addresses", token, map[string]string{
		"recipient_name":  "Legacy",
		"recipient_phone": "01041125167",
		"postal_code":     "06194",
		"address_line1":   "Legacy street",
		"city":            "강남구",
		"province":        "서울특별시",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("legacy POST address: %d %s", rec.Code, rec.Body.String())
	}
	var addr domain.Address
	if err := json.Unmarshal(rec.Body.Bytes(), &addr); err != nil {
		t.Fatal(err)
	}

	rec = doJSON(t, mux, http.MethodGet, "/api/v1/auth/me/addresses", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy GET addresses: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodPost, "/api/v1/auth/me/addresses/"+addr.ID+"/default", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy POST default: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodDelete, "/api/v1/auth/me/addresses/"+addr.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("legacy DELETE address: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAddressNotFound(t *testing.T) {
	mux := newMux(newTestHandler(t))
	token := makeToken(t, "user-1")
	rec := doJSON(t, mux, http.MethodGet, "/api/v1/profile/me/addresses/addr_missing", token, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestInvalidAddressValidation(t *testing.T) {
	mux := newMux(newTestHandler(t))
	token := makeToken(t, "user-1")
	rec := doJSON(t, mux, http.MethodPost, "/api/v1/profile/me/addresses", token, map[string]string{
		"recipient_name":  "Bad",
		"recipient_phone": "12345",
		"postal_code":     "06194",
		"address_line1":   "Line",
		"city":            "강남구",
		"province":        "서울",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid phone: want 400, got %d", rec.Code)
	}
}

func TestAddressLimitReached(t *testing.T) {
	mux := newMux(newTestHandler(t))
	token := makeToken(t, "user-limit")

	for i := 0; i < domain.MaxAddressesPerUser; i++ {
		rec := doJSON(t, mux, http.MethodPost, "/api/v1/profile/me/addresses", token, map[string]string{
			"recipient_name":  "A",
			"recipient_phone": "01011112222",
			"postal_code":     "06194",
			"address_line1":   "Line",
			"city":            "강남구",
			"province":        "서울",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := doJSON(t, mux, http.MethodPost, "/api/v1/profile/me/addresses", token, map[string]string{
		"recipient_name":  "A",
		"recipient_phone": "01011112222",
		"postal_code":     "06194",
		"address_line1":   "Line",
		"city":            "강남구",
		"province":        "서울",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit reached: want 400, got %d %s", rec.Code, rec.Body.String())
	}
}

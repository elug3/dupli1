package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/auth/pkg/domain"
)

func TestProfileAndAddresses_HTTP(t *testing.T) {
	s := newStack(t)
	accessToken := s.registerLoginRefresh(t, "profile@example.com", "supersecret")

	t.Run("empty profile", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me/profile", accessToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET profile: %d %s", w.Code, w.Body.String())
		}
		var view domain.ProfileView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if len(view.Addresses) != 0 {
			t.Fatalf("expected no addresses: %+v", view)
		}
	})

	t.Run("patch profile", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodPatch, "/api/v1/auth/me/profile", accessToken, map[string]string{
			"display_name": "윤라희",
			"phone":        "010-4112-5167",
		})
		if w.Code != http.StatusOK {
			t.Fatalf("PATCH profile: %d %s", w.Code, w.Body.String())
		}
		var view domain.ProfileView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.DisplayName != "윤라희" || view.Phone != "01041125167" {
			t.Fatalf("profile: %+v", view)
		}
	})

	var addressID string
	t.Run("create address", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodPost, "/api/v1/auth/me/addresses", accessToken, map[string]string{
			"recipient_name":  "윤라희",
			"recipient_phone": "01041125167",
			"postal_code":     "06194",
			"address_line1":   "테헤란로 78길 14-12",
			"address_line2":   "9층",
			"city":            "강남구",
			"province":        "서울특별시",
			"label":           "home",
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("POST address: %d %s", w.Code, w.Body.String())
		}
		var addr domain.Address
		if err := json.Unmarshal(w.Body.Bytes(), &addr); err != nil {
			t.Fatal(err)
		}
		if !addr.IsDefault || addr.ID == "" {
			t.Fatalf("address: %+v", addr)
		}
		addressID = addr.ID
	})

	t.Run("get profile includes address", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me/profile", accessToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET profile: %d %s", w.Code, w.Body.String())
		}
		var view domain.ProfileView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.DefaultAddressID != addressID || len(view.Addresses) != 1 {
			t.Fatalf("view: %+v", view)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		w := s.do(t, http.MethodGet, "/api/v1/auth/me/profile", nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", w.Code)
		}
	})

	t.Run("address not found", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me/addresses/addr_missing", accessToken, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", w.Code)
		}
	})

	t.Run("patch address", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodPatch, "/api/v1/auth/me/addresses/"+addressID, accessToken, map[string]string{
			"recipient_name": "Updated Recipient",
			"address_line2":  "10층",
		})
		if w.Code != http.StatusOK {
			t.Fatalf("PATCH address: %d %s", w.Code, w.Body.String())
		}
		var addr domain.Address
		if err := json.Unmarshal(w.Body.Bytes(), &addr); err != nil {
			t.Fatal(err)
		}
		if addr.RecipientName != "Updated Recipient" || addr.AddressLine2 != "10층" {
			t.Fatalf("patched address: %+v", addr)
		}
	})

	t.Run("set default address", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodPost, "/api/v1/auth/me/addresses", accessToken, map[string]string{
			"recipient_name":  "Second",
			"recipient_phone": "01022223333",
			"postal_code":     "06194",
			"address_line1":   "Other street",
			"city":            "강남구",
			"province":        "서울특별시",
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("POST second address: %d %s", w.Code, w.Body.String())
		}
		var second domain.Address
		if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
			t.Fatal(err)
		}

		w = s.doWithAuth(t, http.MethodPost, "/api/v1/auth/me/addresses/"+second.ID+"/default", accessToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("POST default: %d %s", w.Code, w.Body.String())
		}
		var defaulted domain.Address
		if err := json.Unmarshal(w.Body.Bytes(), &defaulted); err != nil {
			t.Fatal(err)
		}
		if !defaulted.IsDefault {
			t.Fatal("second address should be default")
		}

		w = s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me/profile", accessToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET profile: %d %s", w.Code, w.Body.String())
		}
		var view domain.ProfileView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.DefaultAddressID != second.ID {
			t.Fatalf("default_address_id = %q, want %q", view.DefaultAddressID, second.ID)
		}
	})

	t.Run("invalid address validation", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodPost, "/api/v1/auth/me/addresses", accessToken, map[string]string{
			"recipient_name":  "Bad",
			"recipient_phone": "12345",
			"postal_code":     "06194",
			"address_line1":   "Line",
			"city":            "강남구",
			"province":        "서울",
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid phone: want 400, got %d", w.Code)
		}
	})

	t.Run("address pccc round trip", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodPost, "/api/v1/auth/me/addresses", accessToken, map[string]string{
			"recipient_name":  "Customs User",
			"recipient_phone": "01041125167",
			"postal_code":     "06194",
			"address_line1":   "테헤란로 78길 14-12",
			"city":            "강남구",
			"province":        "서울특별시",
			"pccc":            "p123456789012",
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("POST address with pccc: %d %s", w.Code, w.Body.String())
		}
		var addr domain.Address
		if err := json.Unmarshal(w.Body.Bytes(), &addr); err != nil {
			t.Fatal(err)
		}
		if addr.PCCC != "P123456789012" {
			t.Fatalf("created address pccc = %q, want P123456789012", addr.PCCC)
		}

		w = s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me/addresses/"+addr.ID, accessToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET address: %d %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &addr); err != nil {
			t.Fatal(err)
		}
		if addr.PCCC != "P123456789012" {
			t.Fatalf("GET address pccc = %q, want P123456789012", addr.PCCC)
		}

		w = s.doWithAuth(t, http.MethodPost, "/api/v1/auth/me/addresses", accessToken, map[string]string{
			"recipient_name":  "Bad PCCC",
			"recipient_phone": "01041125167",
			"postal_code":     "06194",
			"address_line1":   "Line",
			"city":            "강남구",
			"province":        "서울",
			"pccc":            "invalid",
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid pccc: want 400, got %d", w.Code)
		}
	})

	t.Run("delete address", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodDelete, "/api/v1/auth/me/addresses/"+addressID, accessToken, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("DELETE address: %d %s", w.Code, w.Body.String())
		}
		w = s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me/addresses/"+addressID, accessToken, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("deleted address GET: want 404, got %d", w.Code)
		}
	})
}

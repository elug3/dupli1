package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

// Banner routes sit beside /products/{id}/... patterns; only the full mux
// proves they register without a conflict and carry the right guards.
func TestBannerRoutesThroughBootstrap(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set")
	}
	const secret = "banner-routes-secret"
	app, err := Bootstrap(context.Background(), Config{DatabaseConnString: dsn, JWTSecret: secret})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	sign := func(claims jwt.MapClaims) string {
		claims["type"] = "access"
		claims["exp"] = time.Now().Add(time.Hour).Unix()
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	shopper := sign(jwt.MapClaims{"sub": "c1", "account_type": "customer"})
	staff := sign(jwt.MapClaims{"sub": "m1", "email": "m1@dupli1.com", "account_type": "manager", "permissions": []string{permissions.BannerManage}})

	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		app.Handler.ServeHTTP(rr, req)
		return rr
	}

	if rr := do("GET", "/api/v1/products/banners", "", ""); rr.Code != http.StatusOK {
		t.Fatalf("public live list = %d %s", rr.Code, rr.Body)
	}
	for _, c := range []struct{ method, path, token string }{
		{"GET", "/api/v1/products/banners/all", ""},
		{"GET", "/api/v1/products/banners/all", shopper},
		{"POST", "/api/v1/products/banners", shopper},
		{"POST", "/api/v1/products/banners/images", shopper},
		{"PUT", "/api/v1/products/banners/by-id/x", shopper},
		{"PUT", "/api/v1/products/banners/by-id/x/active", shopper},
		{"DELETE", "/api/v1/products/banners/by-id/x", shopper},
	} {
		if rr := do(c.method, c.path, c.token, "{}"); rr.Code != http.StatusUnauthorized && rr.Code != http.StatusForbidden {
			t.Errorf("%s %s without banner.manage = %d", c.method, c.path, rr.Code)
		}
	}

	rr := do("POST", "/api/v1/products/banners", staff, `{"image_url":"/heroes/x.jpg","title":{"ko":"겨울"},"link_url":"/category/new"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rr.Code, rr.Body)
	}
	var created struct {
		ID        string `json:"id"`
		UpdatedBy string `json:"updated_by"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	t.Cleanup(func() { do("DELETE", "/api/v1/products/banners/by-id/"+created.ID, staff, "") })
	if created.UpdatedBy != "m1@dupli1.com" {
		t.Errorf("updated_by = %q", created.UpdatedBy)
	}
	if rr := do("PUT", "/api/v1/products/banners/by-id/"+created.ID+"/active", staff, `{"active":true}`); rr.Code != http.StatusOK {
		t.Fatalf("activate = %d %s", rr.Code, rr.Body)
	}
	rr = do("GET", "/api/v1/products/banners?placement=home_hero", "", "")
	var live struct {
		Banners []struct{ ID string } `json:"banners"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &live)
	found := false
	for _, b := range live.Banners {
		found = found || b.ID == created.ID
	}
	if !found {
		t.Fatalf("activated banner not in the live list: %s", rr.Body)
	}
	// A product id of "banners" still reaches the product route, not these.
	if rr := do("GET", "/api/v1/products/NO-SUCH", "", ""); rr.Code != http.StatusNotFound {
		t.Errorf("GET /products/NO-SUCH = %d", rr.Code)
	}
}

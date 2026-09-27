package bootstrap

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

// The internal routes are wired here, not in the handler package, so only a
// test against Bootstrap proves a person cannot reach them.
func TestInternalRoutesAcceptOnlyOrderService(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set")
	}
	const secret = "internal-routes-secret"
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
			t.Fatalf("sign: %v", err)
		}
		return s
	}
	callers := []struct {
		name    string
		claims  jwt.MapClaims
		allowed bool
	}{
		{"order service", jwt.MapClaims{"sub": "s1", "account_type": "service", "service_name": "dupli1-order",
			"permissions": []string{permissions.PromotionRedeem, permissions.InventoryReservationManage}}, true},
		{"web service", jwt.MapClaims{"sub": "s2", "account_type": "service", "service_name": "dupli1-web",
			"permissions": []string{permissions.PromotionRedeem, permissions.InventoryReservationManage}}, false},
		{"catalog admin", jwt.MapClaims{"sub": "m1", "account_type": "manager",
			"permissions": []string{permissions.PromotionAll, permissions.InventoryReservationManage}}, false},
		{"owner", jwt.MapClaims{"sub": "o1", "account_type": "manager", "permissions": []string{permissions.All}}, false},
	}
	routes := []string{
		"/api/v1/products/promotions/reserve",
		"/api/v1/products/promotions/consume",
		"/api/v1/products/promotions/release",
		"/api/v1/products/inventory/reservations",
		"/api/v1/products/inventory/reservations/r-1/commit",
		"/api/v1/products/inventory/reservations/r-1/release",
		"/api/v1/inventory/reservations",
	}
	for _, c := range callers {
		token := sign(c.claims)
		for _, route := range routes {
			req := httptest.NewRequest(http.MethodPost, route, bytes.NewBufferString(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rr := httptest.NewRecorder()
			app.Handler.ServeHTTP(rr, req)
			if forbidden := rr.Code == http.StatusForbidden; forbidden == c.allowed {
				t.Errorf("%s POST %s: status %d, allowed=%v", c.name, route, rr.Code, c.allowed)
			}
		}
	}
}

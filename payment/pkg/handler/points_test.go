package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func newPointsMux(t *testing.T) (*http.ServeMux, *memory.Points) {
	t.Helper()
	store := memory.NewPoints()
	svc := service.New(memory.NewRepository(), stubOrderClient{}, fakeCheckoutProvider{}, nil)
	h := handler.New(svc, authjwt.NewHMACValidator("test-secret")).WithPoints(service.NewPoints(store))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, store
}

func accountToken(t *testing.T, sub, accountType, serviceName string, perms []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub, "permissions": perms, "type": "access",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"account_type": accountType,
	}
	if serviceName != "" {
		claims["service_name"] = serviceName
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func grant(t *testing.T, store *memory.Points, customer string, points int64) {
	t.Helper()
	if _, err := store.MutatePointAccount(context.Background(), customer, func(a *domain.PointAccount) error {
		_, err := a.Grant(domain.PointSourceAdjust, "seed-"+customer, points, "seed", time.Now().UTC())
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPointsMeShowsOnlyOwnAccount(t *testing.T) {
	mux, store := newPointsMux(t)
	grant(t, store, "u-1", 5_000)
	grant(t, store, "u-2", 9_000)

	rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/points/me", accountToken(t, "u-1", "customer", "", nil), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}
	var s service.PointsSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if s.CustomerID != "u-1" || s.Balance != 5_000 || s.Tier.Code != "member" || len(s.Entries) != 1 {
		t.Fatalf("summary = %+v", s)
	}

	if rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/points/me", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/points/customers/u-2", accountToken(t, "u-1", "customer", "", nil), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("customer reading another: %d", rec.Code)
	}
}

func TestManagerReadsAndAdjustsPoints(t *testing.T) {
	mux, store := newPointsMux(t)
	grant(t, store, "u-1", 1_000)
	reader := accountToken(t, "m-1", "manager", "", []string{permissions.PaymentPointsRead})
	manager := accountToken(t, "m-2", "manager", "", []string{permissions.PaymentPointsManage})

	if rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/points/customers/u-1", reader, nil); rec.Code != http.StatusOK {
		t.Fatalf("read: %d %s", rec.Code, rec.Body.String())
	}
	body := map[string]any{"points": 2_000, "reason": "service recovery"}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/customers/u-1/adjust", reader, body); rec.Code != http.StatusForbidden {
		t.Fatalf("reader adjusting: %d", rec.Code)
	}
	rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/customers/u-1/adjust", manager, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("adjust: %d %s", rec.Code, rec.Body.String())
	}
	var s service.PointsSummary
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Balance != 3_000 {
		t.Fatalf("balance = %d, want 3000", s.Balance)
	}
	rec = doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/customers/u-1/adjust", manager, map[string]any{"points": -5_000, "reason": "typo"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("over-deduct: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/customers/u-1/adjust", manager, map[string]any{"points": 10}); rec.Code != http.StatusBadRequest {
		t.Fatalf("no reason: %d", rec.Code)
	}
}

func TestPointsHoldsAreForOrderServiceOnly(t *testing.T) {
	mux, store := newPointsMux(t)
	grant(t, store, "u-1", 10_000)
	order := accountToken(t, "svc-order", "service", "dupli1-order", []string{permissions.PaymentPointsRedeem})
	web := accountToken(t, "svc-web", "service", "dupli1-web", []string{permissions.PaymentPointsRedeem})
	owner := accountToken(t, "owner", "manager", "", []string{permissions.All})
	reserve := map[string]any{"order_id": "o1", "customer_id": "u-1", "points": 4_000}

	for name, tok := range map[string]string{"web service": web, "owner": owner} {
		if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/reserve", tok, reserve); rec.Code != http.StatusForbidden {
			t.Fatalf("%s reserving: %d", name, rec.Code)
		}
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/reserve", order, reserve); rec.Code != http.StatusOK {
		t.Fatalf("reserve: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/reserve", order,
		map[string]any{"order_id": "o2", "customer_id": "u-1", "points": 7_000}); rec.Code != http.StatusConflict {
		t.Fatalf("over-reserve: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/consume", order, map[string]any{"order_id": "o1"}); rec.Code != http.StatusOK {
		t.Fatalf("consume: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/release", order, map[string]any{"order_id": "o1"}); rec.Code != http.StatusOK {
		t.Fatalf("release: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/payments/points/consume", order, map[string]any{"order_id": "nope"}); rec.Code != http.StatusNotFound {
		t.Fatalf("consume unknown: %d", rec.Code)
	}

	rec := doJSON(t, mux, http.MethodGet, "/api/v1/payments/points/me", accountToken(t, "u-1", "customer", "", nil), nil)
	var s service.PointsSummary
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Balance != 10_000 {
		t.Fatalf("balance after spend and return = %d, want 10000", s.Balance)
	}
}

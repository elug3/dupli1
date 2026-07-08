package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/authjwt"
	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/handler"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "handler-test-secret"

// makeToken creates a signed JWT for the given identity.
func makeToken(t *testing.T, userID string, perms []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":         userID,
		"type":        "access",
		"permissions": perms,
		"exp":         time.Now().Add(time.Hour).Unix(),
		"iat":         time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return signed
}

// bearerHeader returns an Authorization header value.
func bearerHeader(token string) string { return "Bearer " + token }

type fakeInventory struct{}

func (f *fakeInventory) Reserve(_ context.Context, _ string, _ []ports.InventoryItem) (string, error) {
	return "res-001", nil
}
func (f *fakeInventory) CommitReservation(_ context.Context, _ string) error  { return nil }
func (f *fakeInventory) ReleaseReservation(_ context.Context, _ string) error { return nil }

func newTestHandler(t *testing.T) (*handler.Handler, *service.Service) {
	t.Helper()
	repo := memory.NewRepository()
	svc := service.New(repo, &fakeInventory{})
	validator := authjwt.NewHMACValidator(testSecret)
	return handler.New(svc, validator), svc
}

func newMux(h *handler.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// createOrder via the service and return the order ID.
func seedOrder(t *testing.T, svc *service.Service, customerID string) string {
	t.Helper()
	order, err := svc.CreateOrder(context.Background(), service.CreateOrderInput{
		CustomerID: customerID,
		Items:      []domain.OrderItem{{SKU: "ITEM-1", Quantity: 1, UnitPriceCents: 1000}},
	})
	if err != nil {
		t.Fatalf("seedOrder: %v", err)
	}
	return order.ID
}

// do sends a request against the given mux and returns the response.
func do(t *testing.T, mux *http.ServeMux, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &b)
	if token != "" {
		req.Header.Set("Authorization", bearerHeader(token))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// ── Auth middleware ───────────────────────────────────────────────────────────

func TestRequireAuth_NoToken_Returns401(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	// Hit an authenticated endpoint without a token.
	w := do(t, mux, http.MethodGet, "/api/v1/orders?customer_id=x", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRequireAuth_MalformedToken_Returns401(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders?customer_id=x", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRequireAuth_ValidToken_Passes(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedOrder(t, svc, "u-1")
	token := makeToken(t, "u-1", nil)

	w := do(t, mux, http.MethodGet, fmt.Sprintf("/api/v1/orders/%s", orderID), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHealthEndpoint_DoesNotRequireAuth(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	w := do(t, mux, http.MethodGet, "/api/v1/orders/health", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ── POST /api/v1/orders ───────────────────────────────────────────────────────

func TestCreateOrder_CustomerCanCreateOwnOrder(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	token := makeToken(t, "u-1", nil)

	body := map[string]any{
		"customer_id": "u-1",
		"items":       []map[string]any{{"sku": "SHOE-1", "quantity": 1, "unit_price_cents": 999}},
	}
	w := do(t, mux, http.MethodPost, "/api/v1/orders", token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

func TestCreateOrder_CustomerForbiddenOnOthersCustomerID(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	token := makeToken(t, "u-1", nil)

	body := map[string]any{
		"customer_id": "u-2", // different from token subject
		"items":       []map[string]any{{"sku": "SHOE-1", "quantity": 1, "unit_price_cents": 999}},
	}
	w := do(t, mux, http.MethodPost, "/api/v1/orders", token, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestCreateOrder_OrderManagerCannotCreateForOtherCustomer(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	token := makeToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}))

	body := map[string]any{
		"customer_id": "u-99",
		"items":       []map[string]any{{"sku": "SHOE-1", "quantity": 1, "unit_price_cents": 999}},
	}
	w := do(t, mux, http.MethodPost, "/api/v1/orders", token, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestCreateOrder_AdminCanCreateForAnyCustomer(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	token := makeToken(t, "admin-1", permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin}))

	body := map[string]any{
		"customer_id": "u-99",
		"items":       []map[string]any{{"sku": "SHOE-1", "quantity": 1, "unit_price_cents": 999}},
	}
	w := do(t, mux, http.MethodPost, "/api/v1/orders", token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

// ── GET /api/v1/orders?customer_id ───────────────────────────────────────────

func TestListOrders_CustomerCanListOwnOrders(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	token := makeToken(t, "u-1", nil)

	w := do(t, mux, http.MethodGet, "/api/v1/orders?customer_id=u-1", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestListOrders_CustomerForbiddenOnOthersOrders(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	token := makeToken(t, "u-1", nil)

	w := do(t, mux, http.MethodGet, "/api/v1/orders?customer_id=u-2", token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestListOrders_OrderManagerCanListAny(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	token := makeToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}))

	w := do(t, mux, http.MethodGet, "/api/v1/orders?customer_id=u-99", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ── GET /api/v1/orders/{id} ───────────────────────────────────────────────────

func TestGetOrder_CustomerCanReadOwnOrder(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedOrder(t, svc, "u-1")
	token := makeToken(t, "u-1", nil)

	w := do(t, mux, http.MethodGet, "/api/v1/orders/"+orderID, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestGetOrder_CustomerForbiddenOnOthersOrder(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedOrder(t, svc, "u-1")     // order owned by u-1
	token := makeToken(t, "u-2", nil) // logged in as u-2

	w := do(t, mux, http.MethodGet, "/api/v1/orders/"+orderID, token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestGetOrder_OrderManagerCanReadAnyOrder(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedOrder(t, svc, "u-1")
	token := makeToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}))

	w := do(t, mux, http.MethodGet, "/api/v1/orders/"+orderID, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestGetOrder_AdminCanReadAnyOrder(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedOrder(t, svc, "u-1")
	token := makeToken(t, "admin-1", permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin}))

	w := do(t, mux, http.MethodGet, "/api/v1/orders/"+orderID, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ── PUT /api/v1/orders/{id}/status ───────────────────────────────────────────

func TestUpdateStatus_CustomerForbidden(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedOrder(t, svc, "u-1")
	token := makeToken(t, "u-1", nil)

	body := map[string]string{"status": "fulfilled"}
	w := do(t, mux, http.MethodPut, "/api/v1/orders/"+orderID+"/status", token, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func seedPaidOrder(t *testing.T, svc *service.Service, customerID string) string {
	t.Helper()
	orderID := seedOrder(t, svc, customerID)
	order, err := svc.GetOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if _, err := svc.MarkOrderPaid(context.Background(), orderID, "pay-test", order.TotalCents); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	return orderID
}

func TestShipOrder_OrderManagerSuccess(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedPaidOrder(t, svc, "u-1")
	token := makeToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}))

	w := do(t, mux, http.MethodPost, "/api/v1/orders/"+orderID+"/ship", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestShipOrder_CustomerForbidden(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedPaidOrder(t, svc, "u-1")
	token := makeToken(t, "u-1", nil)

	w := do(t, mux, http.MethodPost, "/api/v1/orders/"+orderID+"/ship", token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestUpdateStatus_ConfirmedRejected(t *testing.T) {
	h, svc := newTestHandler(t)
	mux := newMux(h)

	orderID := seedOrder(t, svc, "u-1")
	token := makeToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}))

	body := map[string]string{"status": "confirmed"}
	w := do(t, mux, http.MethodPut, "/api/v1/orders/"+orderID+"/status", token, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

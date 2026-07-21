package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// AccessTokenValidator validates Bearer access tokens and returns claims.
type AccessTokenValidator interface {
	ValidateAccessToken(token string) (authjwt.Claims, error)
}

type Handler struct {
	svc          *service.Service
	jwtValidator AccessTokenValidator
	settings     settings.Response
}

func New(svc *service.Service, jwtValidator AccessTokenValidator) *Handler {
	return &Handler{
		svc:          svc,
		jwtValidator: jwtValidator,
		settings:     settings.NewResponse("order"),
	}
}

// WithSettings sets the non-secret settings payload served by GET /settings.
func (h *Handler) WithSettings(s settings.Response) *Handler {
	h.settings = s
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/orders/health", h.health)
	mux.HandleFunc("/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/orders/settings", h.settingsHandler)
	// Checkout before /api/v1/orders/ catch-all so it is not shadowed.
	mux.HandleFunc("/api/v1/orders/checkout/sessions", h.requireAuth(h.checkoutSessions))
	mux.HandleFunc("/api/v1/orders/checkout/sessions/", h.requireAuth(h.checkoutSession))
	mux.HandleFunc("/api/v1/checkout/sessions", h.requireAuth(h.checkoutSessions))
	mux.HandleFunc("/api/v1/checkout/sessions/", h.requireAuth(h.checkoutSession))
	mux.HandleFunc("/api/v1/orders", h.requireAuth(h.orders))
	mux.HandleFunc("/api/v1/orders/", h.requireAuth(h.order))
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) settingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	respondJSON(w, http.StatusOK, h.settings)
}

// requireAuth extracts and validates the Bearer token, stores claims in context.
// Fails closed when no validator is configured (misconfigured deploy).
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.jwtValidator == nil {
			respondError(w, http.StatusServiceUnavailable, "auth not configured")
			return
		}

		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || !strings.EqualFold(authHeader[:7], "bearer ") {
			respondError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}

		claims, err := h.jwtValidator.ValidateAccessToken(authHeader[7:])
		if err != nil {
			respondError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		next(w, r.WithContext(authjwt.WithClaims(r.Context(), claims)))
	}
}

func (h *Handler) orders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createOrder(w, r)
	case http.MethodGet:
		h.listOrders(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())

	var req struct {
		CustomerID string `json:"customer_id"`
		Items      []struct {
			SkuID    string `json:"sku_id"`
			SKU      string `json:"sku"`
			Quantity int    `json:"quantity"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ABAC: storefront callers may only create orders for themselves.
	if h.jwtValidator != nil && !permissions.BypassesOrderCreateABAC(claims.Permissions) {
		if req.CustomerID != claims.UserID {
			respondError(w, http.StatusForbidden, "forbidden: customer_id must match your user id")
			return
		}
	}

	items := make([]domain.OrderItem, len(req.Items))
	for i, item := range req.Items {
		items[i] = domain.OrderItem{SkuID: item.SkuID, SKU: item.SKU, Quantity: item.Quantity}
	}

	order, err := h.svc.CreateOrder(r.Context(), service.CreateOrderInput{
		CustomerID:     req.CustomerID,
		Items:          items,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, order)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())

	customerID := r.URL.Query().Get("customer_id")
	if customerID == "" {
		respondError(w, http.StatusBadRequest, "customer_id query parameter is required")
		return
	}

	// ABAC: storefront callers may only list their own orders.
	if h.jwtValidator != nil && !permissions.BypassesOrderReadABAC(claims.Permissions) {
		if customerID != claims.UserID {
			respondError(w, http.StatusForbidden, "forbidden: can only list your own orders")
			return
		}
	}

	orders, err := h.svc.ListCustomerOrders(r.Context(), customerID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"total":  len(orders),
		"orders": orders,
	})
}

func (h *Handler) order(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/v1/orders/"))
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		h.getOrder(w, r, parts[0])
		return
	}

	if len(parts) == 2 && parts[1] == "ship" && r.Method == http.MethodPost {
		h.shipOrder(w, r, parts[0])
		return
	}

	if len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodPut {
		h.updateStatus(w, r, parts[0])
		return
	}

	respondError(w, http.StatusNotFound, "not found")
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request, orderID string) {
	claims, _ := authjwt.FromContext(r.Context())

	order, err := h.svc.GetOrder(r.Context(), orderID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	// ABAC: storefront callers may only read their own orders.
	if h.jwtValidator != nil && !permissions.BypassesOrderReadABAC(claims.Permissions) {
		if order.CustomerID != claims.UserID {
			respondError(w, http.StatusForbidden, "forbidden: you do not own this order")
			return
		}
	}

	respondJSON(w, http.StatusOK, order)
}

func (h *Handler) shipOrder(w http.ResponseWriter, r *http.Request, orderID string) {
	claims, _ := authjwt.FromContext(r.Context())

	if h.jwtValidator != nil && !claims.HasPermission(permissions.OrderShip) {
		respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
		return
	}

	order, err := h.svc.ShipOrder(r.Context(), orderID, claims.UserID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, order)
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request, orderID string) {
	claims, _ := authjwt.FromContext(r.Context())

	if h.jwtValidator != nil && !claims.HasPermission(permissions.OrderStatusUpdate) {
		respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
		return
	}

	var req struct {
		Status domain.OrderStatus `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var (
		order *domain.Order
		err   error
	)
	switch req.Status {
	case domain.StatusCanceled:
		order, err = h.svc.CancelOrder(r.Context(), orderID)
	case domain.StatusFulfilled:
		order, err = h.svc.FulfillOrder(r.Context(), orderID)
	default:
		respondError(w, http.StatusBadRequest, "unsupported status; use POST /ship for in_transit")
		return
	}
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, order)
}

func respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ports.ErrIdempotencyConflict):
		respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ports.ErrVariantNotFound):
		respondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ports.ErrProductUnavailable), errors.Is(err, ports.ErrCouponUnavailable):
		respondError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, domain.ErrInvalidOrder), errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrPaymentAmountMismatch),
		errors.Is(err, domain.ErrInvalidCheckoutSession), errors.Is(err, domain.ErrEmptyCheckout),
		errors.Is(err, domain.ErrSessionNotOpen):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("order: internal error: %v", err)
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]any{
		"error": message,
		"code":  status,
	})
}

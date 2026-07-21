package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/cart/pkg/authjwt"
	"github.com/elug3/dupli1/cart/pkg/domain"
	"github.com/elug3/dupli1/cart/pkg/ports"
	"github.com/elug3/dupli1/cart/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

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
		settings:     settings.NewResponse("cart"),
	}
}

// WithSettings sets the non-secret settings payload served by GET /settings.
func (h *Handler) WithSettings(s settings.Response) *Handler {
	h.settings = s
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/cart/health", h.health)
	mux.HandleFunc("/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/cart/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/cart", h.requireAuth(h.cart))
	mux.HandleFunc("/api/v1/cart/items", h.requireAuth(h.cartItems))
	mux.HandleFunc("/api/v1/cart/items/", h.requireAuth(h.cartItem))
	// Canonical admin cart under the cart service prefix.
	mux.HandleFunc("/api/v1/cart/customers/", h.requireAuth(h.adminCart))
	// Legacy alias.
	mux.HandleFunc("/api/v1/carts/", h.requireAuth(h.adminCart))
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

func (h *Handler) cart(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		cart, err := h.svc.GetCart(r.Context(), claims.UserID)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, cart)
	case http.MethodDelete:
		if err := h.svc.ClearCart(r.Context(), claims.UserID); err != nil {
			respondServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) cartItems(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())

	switch r.Method {
	case http.MethodPut:
		var req struct {
			Items []struct {
				SkuID    string `json:"sku_id"`
				SKU      string `json:"sku"`
				Quantity int    `json:"quantity"`
			} `json:"items"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		inputs := make([]service.ItemInput, len(req.Items))
		for i, item := range req.Items {
			inputs[i] = service.ItemInput{SkuID: item.SkuID, SKU: item.SKU, Quantity: item.Quantity}
		}
		cart, err := h.svc.ReplaceItems(r.Context(), claims.UserID, inputs)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, cart)
	case http.MethodPost:
		var req struct {
			SkuID    string `json:"sku_id"`
			SKU      string `json:"sku"`
			Quantity int    `json:"quantity"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		cart, err := h.svc.UpsertItem(r.Context(), claims.UserID, service.ItemInput{
			SkuID:    req.SkuID,
			SKU:      req.SKU,
			Quantity: req.Quantity,
		})
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, cart)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) cartItem(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())

	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/v1/cart/items/"))

	var (
		cart *domain.Cart
		err  error
	)
	switch {
	case len(parts) == 2 && parts[0] == "by-sku-id" && parts[1] != "":
		cart, err = h.svc.RemoveItemBySkuID(r.Context(), claims.UserID, parts[1])
	case len(parts) == 1 && parts[0] != "":
		cart, err = h.svc.RemoveItem(r.Context(), claims.UserID, parts[0])
	default:
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, cart)
}

func (h *Handler) adminCart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, _ := authjwt.FromContext(r.Context())
	if h.jwtValidator != nil && !claims.HasPermission(permissions.CartRead) {
		respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
		return
	}

	parts := adminCartPathParts(r.URL.Path)
	if len(parts) != 1 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	cart, err := h.svc.GetCart(r.Context(), parts[0])
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, cart)
}

func respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrVariantNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ports.ErrProductUnavailable):
		respondError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, domain.ErrInvalidCart), errors.Is(err, domain.ErrInvalidCartItem):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, err.Error())
	}
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func adminCartPathParts(path string) []string {
	for _, prefix := range []string{
		"/api/v1/cart/customers/",
		"/api/v1/carts/",
	} {
		if strings.HasPrefix(path, prefix) {
			return splitPath(strings.TrimPrefix(path, prefix))
		}
	}
	return nil
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

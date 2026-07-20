package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/payment/pkg/authjwt"
	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/stripe/stripe-go/v81/webhook"
)

type AccessTokenValidator interface {
	ValidateAccessToken(token string) (authjwt.Claims, error)
}

type Handler struct {
	svc              *service.Service
	jwtValidator     AccessTokenValidator
	webhookSecret    string
	allowDevSimulate bool
	settings         settings.Response
}

func New(svc *service.Service, jwtValidator AccessTokenValidator, webhookSecret string) *Handler {
	return &Handler{
		svc:           svc,
		jwtValidator:  jwtValidator,
		webhookSecret: webhookSecret,
		settings:      settings.NewResponse("payment"),
	}
}

// WithDevSimulate enables GET /api/v1/payments/{id}/simulate-success (local/dev only).
func (h *Handler) WithDevSimulate(allow bool) *Handler {
	h.allowDevSimulate = allow
	return h
}

// WithSettings sets the non-secret settings payload served by GET /settings.
func (h *Handler) WithSettings(s settings.Response) *Handler {
	h.settings = s
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/payments/health", h.health)
	mux.HandleFunc("/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/payments/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/payments", h.requireAuth(h.payments))
	mux.HandleFunc("/api/v1/payments/", h.paymentRoutes)
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

func (h *Handler) payments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	var req struct {
		OrderID string `json:"order_id"`
		Method  string `json:"method"`
		Note    string `json:"note"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	bearerToken := ""
	if auth := r.Header.Get("Authorization"); len(auth) > 7 {
		bearerToken = auth[7:]
	}
	payment, err := h.svc.CreatePayment(r.Context(), service.CreatePaymentInput{
		OrderID:           req.OrderID,
		CustomerID:        claims.UserID,
		BearerToken:       bearerToken,
		IdempotencyKey:    r.Header.Get("Idempotency-Key"),
		Method:            req.Method,
		Note:              req.Note,
		CreatedBy:         claims.UserID,
		BypassABAC:        permissions.BypassesPaymentCreateABAC(claims.Permissions),
		AllowMethodBypass: permissions.CanBypassPayment(claims.Permissions),
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, payment)
}

func (h *Handler) paymentRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/payments/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	if parts[0] == "webhooks" && len(parts) == 2 && parts[1] == "stripe" && r.Method == http.MethodPost {
		h.stripeWebhook(w, r)
		return
	}

	if len(parts) == 2 && parts[1] == "simulate-success" && r.Method == http.MethodGet {
		if !h.allowDevSimulate {
			respondError(w, http.StatusNotFound, "not found")
			return
		}
		h.simulateSuccess(w, r, parts[0])
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
			h.getPayment(w, r, parts[0])
		})(w, r)
		return
	}

	respondError(w, http.StatusNotFound, "not found")
}

func (h *Handler) getPayment(w http.ResponseWriter, r *http.Request, paymentID string) {
	claims, _ := authjwt.FromContext(r.Context())
	ownerID := claims.UserID
	if h.jwtValidator != nil && permissions.BypassesPaymentReadABAC(claims.Permissions) {
		ownerID = ""
	}
	payment, err := h.svc.GetPayment(r.Context(), paymentID, ownerID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, payment)
}

func (h *Handler) simulateSuccess(w http.ResponseWriter, r *http.Request, paymentID string) {
	payment, err := h.svc.CompletePayment(r.Context(), paymentID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Payment received — we're preparing your order.",
		"payment": payment,
	})
}

func (h *Handler) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if h.webhookSecret == "" {
		respondError(w, http.StatusServiceUnavailable, "stripe webhook not configured")
		return
	}
	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), h.webhookSecret)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid signature")
		return
	}
	if event.Type != "checkout.session.completed" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var sess struct {
		ID          string            `json:"id"`
		Metadata    map[string]string `json:"metadata"`
		AmountTotal int64             `json:"amount_total"`
	}
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		respondError(w, http.StatusBadRequest, "invalid session payload")
		return
	}
	if err := h.svc.HandleStripeCheckoutCompleted(r.Context(), sess.ID, sess.Metadata["order_id"], sess.Metadata["payment_id"], sess.AmountTotal); err != nil {
		respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound), errors.Is(err, ports.ErrOrderNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ports.ErrOrderForbidden), errors.Is(err, ports.ErrPaymentForbidden):
		respondError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ports.ErrMethodUnavailable):
		respondError(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, ports.ErrOrderNotPending), errors.Is(err, domain.ErrInvalidPayment):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, err.Error())
	}
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
	respondJSON(w, status, map[string]any{"error": message, "code": status})
}

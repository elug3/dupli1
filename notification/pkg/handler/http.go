package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/jackc/pgx/v4"
)

type AccessTokenValidator interface {
	ValidateAccessToken(token string) (authjwt.Claims, error)
}

type Handler struct {
	telegramSubs           *service.TelegramSubscriptions
	updateProcessor        *telegram.UpdateProcessor
	webhookSecret            string
	jwtValidator             AccessTokenValidator
	settings                 settings.Response
	onSubscriptionsChanged func()
}

type Options struct {
	TelegramSubs           *service.TelegramSubscriptions
	UpdateProcessor        *telegram.UpdateProcessor
	WebhookSecret          string
	JWTValidator           AccessTokenValidator
	Settings               settings.Response
	OnSubscriptionsChanged func()
}

func New(opts Options) *Handler {
	return &Handler{
		telegramSubs:           opts.TelegramSubs,
		updateProcessor:        opts.UpdateProcessor,
		webhookSecret:          strings.TrimSpace(opts.WebhookSecret),
		jwtValidator:           opts.JWTValidator,
		settings:               opts.Settings,
		onSubscriptionsChanged: opts.OnSubscriptionsChanged,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/notification/health", h.health)
	mux.HandleFunc("/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/notification/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/notification/telegram/webhook", h.telegramWebhook)
	mux.HandleFunc("/api/v1/notification/telegram/subscriptions", h.requireAuth(h.telegramSubscriptions))
	mux.HandleFunc("/api/v1/notification/telegram/subscriptions/", h.requireAuth(h.telegramSubscriptionAction))
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

func (h *Handler) telegramWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.updateProcessor == nil {
		respondError(w, http.StatusServiceUnavailable, "telegram webhook not configured")
		return
	}
	if h.webhookSecret == "" {
		respondError(w, http.StatusServiceUnavailable, "webhook secret not configured")
		return
	}
	if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != h.webhookSecret {
		respondError(w, http.StatusForbidden, "invalid webhook secret")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}

	var update telegram.Update
	if err := json.Unmarshal(body, &update); err != nil {
		respondError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}

	if err := h.updateProcessor.Handle(r.Context(), update); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to process update")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) telegramSubscriptions(w http.ResponseWriter, r *http.Request) {
	if h.telegramSubs == nil || !h.telegramSubs.Enabled() {
		respondError(w, http.StatusServiceUnavailable, "telegram subscriptions not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !h.canRead(r) {
			respondError(w, http.StatusForbidden, "forbidden")
			return
		}
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		items, err := h.telegramSubs.List(r.Context(), status)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list subscriptions")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		if !h.canManage(r) {
			respondError(w, http.StatusForbidden, "forbidden")
			return
		}
		var req struct {
			TelegramUserID *int64 `json:"telegram_user_id"`
			ChatID         string `json:"chat_id"`
			ChatLabel      string `json:"chat_label"`
			AlertOrder     bool   `json:"alert_order"`
			AlertProduct   bool   `json:"alert_product"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid json")
			return
		}
		claims, _ := authjwt.FromContext(r.Context())
		item, err := h.telegramSubs.CreateManual(r.Context(), ports.TelegramManualInput{
			TelegramUserID: req.TelegramUserID,
			ChatID:         req.ChatID,
			ChatLabel:      req.ChatLabel,
			AlertOrder:     req.AlertOrder,
			AlertProduct:   req.AlertProduct,
			AcceptedBy:     claims.UserID,
		})
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.notifyChanged()
		respondJSON(w, http.StatusCreated, item)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) telegramSubscriptionAction(w http.ResponseWriter, r *http.Request) {
	if h.telegramSubs == nil || !h.telegramSubs.Enabled() {
		respondError(w, http.StatusServiceUnavailable, "telegram subscriptions not configured")
		return
	}
	if !h.canManage(r) {
		respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/notification/telegram/subscriptions/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	claims, _ := authjwt.FromContext(r.Context())

	switch action {
	case "accept":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			AlertOrder   bool `json:"alert_order"`
			AlertProduct bool `json:"alert_product"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		item, err := h.telegramSubs.Accept(r.Context(), id, ports.TelegramAcceptInput{
			AlertOrder:   req.AlertOrder,
			AlertProduct: req.AlertProduct,
			AcceptedBy:   claims.UserID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				respondError(w, http.StatusNotFound, "subscription not found")
				return
			}
			respondError(w, http.StatusInternalServerError, "failed to accept subscription")
			return
		}
		h.notifyChanged()
		respondJSON(w, http.StatusOK, item)
	case "reject":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		item, err := h.telegramSubs.Reject(r.Context(), id, claims.UserID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				respondError(w, http.StatusNotFound, "subscription not found")
				return
			}
			respondError(w, http.StatusInternalServerError, "failed to reject subscription")
			return
		}
		h.notifyChanged()
		respondJSON(w, http.StatusOK, item)
	case "":
		if r.Method != http.MethodDelete {
			respondError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := h.telegramSubs.Delete(r.Context(), id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				respondError(w, http.StatusNotFound, "subscription not found")
				return
			}
			respondError(w, http.StatusInternalServerError, "failed to delete subscription")
			return
		}
		h.notifyChanged()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) notifyChanged() {
	if h.onSubscriptionsChanged != nil {
		h.onSubscriptionsChanged()
	}
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

func (h *Handler) canRead(r *http.Request) bool {
	claims, ok := authjwt.FromContext(r.Context())
	if !ok {
		return false
	}
	return claims.HasPermission(permissions.NotificationTelegramRead, permissions.NotificationTelegramManage, permissions.All, permissions.AdminAll)
}

func (h *Handler) canManage(r *http.Request) bool {
	claims, ok := authjwt.FromContext(r.Context())
	if !ok {
		return false
	}
	return claims.HasPermission(permissions.NotificationTelegramManage, permissions.All, permissions.AdminAll)
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

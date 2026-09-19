package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/authmiddleware"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	tg "github.com/elug3/dupli1/shared/pkg/telegram"
	"github.com/jackc/pgx/v4"
)

const (
	// webhookProcessTimeout bounds the work an acknowledged update may do. It
	// outlives the request deliberately — the HTTP response is already sent.
	webhookProcessTimeout = 30 * time.Second
	// healthProbeTimeout keeps a stuck dependency from hanging /health.
	healthProbeTimeout = 2 * time.Second
	// healthCacheTTL bounds how often an unauthenticated request can reach a
	// dependency: without it, /health is a free way to make this service ping
	// its database as fast as anyone can ask.
	healthCacheTTL = 5 * time.Second
)

// HealthProbe reports whether a dependency is currently usable.
type HealthProbe func(ctx context.Context) error

// dependencyStatus is the per-dependency health payload. It carries no error
// text: /health is unauthenticated, and a connection failure's message tends to
// name hosts and users. The cause is logged instead.
type dependencyStatus struct {
	OK bool `json:"ok"`
}

type Handler struct {
	telegramSubs           *service.TelegramSubscriptions
	updateProcessor        *telegram.UpdateProcessor
	webhookSecret          string
	jwtValidator           authjwt.AccessTokenValidator
	settings               settings.Response
	onSubscriptionsChanged func()
	updateCtx              context.Context

	healthProbes   map[string]HealthProbe
	healthMu       sync.Mutex
	healthAt       time.Time
	healthDeps     map[string]dependencyStatus
	healthDegraded bool
}

type Options struct {
	TelegramSubs           *service.TelegramSubscriptions
	UpdateProcessor        *telegram.UpdateProcessor
	WebhookSecret          string
	JWTValidator           authjwt.AccessTokenValidator
	Settings               settings.Response
	OnSubscriptionsChanged func()
	// UpdateContext is the root for work that continues after a webhook has
	// been acknowledged; it is cancelled on shutdown. Defaults to
	// context.Background().
	UpdateContext context.Context
	// HealthProbes are the dependencies /health reports on, by name. A
	// dependency the service does not use is simply absent.
	HealthProbes map[string]HealthProbe
}

func New(opts Options) *Handler {
	updateCtx := opts.UpdateContext
	if updateCtx == nil {
		updateCtx = context.Background()
	}
	return &Handler{
		telegramSubs:           opts.TelegramSubs,
		updateProcessor:        opts.UpdateProcessor,
		webhookSecret:          strings.TrimSpace(opts.WebhookSecret),
		jwtValidator:           opts.JWTValidator,
		settings:               opts.Settings,
		onSubscriptionsChanged: opts.OnSubscriptionsChanged,
		updateCtx:              updateCtx,
		healthProbes:           opts.HealthProbes,
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

// health reports liveness plus a cached view of each dependency.
//
// The status code stays 200 whatever the probes say. Nothing consumes this
// endpoint today — the notification container declares no ECS health check and
// sits behind Cloud Map rather than an ALB target group — so a 503 would signal
// nothing to anyone, while setting up a restart loop for whoever later wires a
// probe to it during a NATS blip. The body carries the detail; a caller that
// wants to act on it can read "status".
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body := map[string]any{"status": "ok"}
	if deps, degraded := h.dependencyStatus(); len(deps) > 0 {
		body["dependencies"] = deps
		if degraded {
			body["status"] = "degraded"
		}
	}
	respondJSON(w, http.StatusOK, body)
}

// dependencyStatus runs the probes, at most once per healthCacheTTL.
//
// The probes run under the service's context, not the request's: a client that
// disconnects mid-probe would otherwise have its cancellation cached as a
// dependency failure. Holding the lock across the probes is deliberate too —
// concurrent requests wait for one round rather than starting their own.
func (h *Handler) dependencyStatus() (map[string]dependencyStatus, bool) {
	if len(h.healthProbes) == 0 {
		return nil, false
	}

	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	if h.healthDeps != nil && time.Since(h.healthAt) < healthCacheTTL {
		return h.healthDeps, h.healthDegraded
	}

	ctx, cancel := context.WithTimeout(h.updateCtx, healthProbeTimeout)
	defer cancel()

	deps := make(map[string]dependencyStatus, len(h.healthProbes))
	degraded := false
	for name, probe := range h.healthProbes {
		err := probe(ctx)
		deps[name] = dependencyStatus{OK: err == nil}
		if err != nil {
			degraded = true
			log.Printf("notification health: %s is unhealthy: %v", name, err)
		}
	}

	h.healthDeps, h.healthDegraded, h.healthAt = deps, degraded, time.Now()
	return deps, degraded
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
	presented := []byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))
	if subtle.ConstantTimeCompare(presented, []byte(h.webhookSecret)) != 1 {
		respondError(w, http.StatusForbidden, "invalid webhook secret")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}

	var update tg.Update
	if err := json.Unmarshal(body, &update); err != nil {
		respondError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}

	// Acknowledge first, then work. Processing an update means a database write
	// and a Bot API round trip, which can outlast the server's WriteTimeout;
	// the cut-off response then had Telegram redeliver an update already being
	// handled. The trade-off is that a failure after this point is a log line
	// rather than a Telegram retry — the sender can always /start again.
	go h.processUpdate(update)
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
			AlertSupport   bool   `json:"alert_support"`
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
			AlertSupport:   req.AlertSupport,
			AlertProduct:   req.AlertProduct,
			AcceptedBy:     claims.UserID,
		})
		switch {
		case err == nil:
		case errors.Is(err, service.ErrIdentifierRequired):
			respondError(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, ports.ErrDuplicateSubscription):
			respondError(w, http.StatusConflict, "subscription already exists")
			return
		default:
			// Never echo the driver's text: a constraint name is not something
			// the caller can act on, and a store failure is not a bad request.
			respondError(w, http.StatusInternalServerError, "failed to create subscription")
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
			AlertSupport bool `json:"alert_support"`
			AlertProduct bool `json:"alert_product"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		item, err := h.telegramSubs.Accept(r.Context(), id, ports.TelegramAcceptInput{
			AlertOrder:   req.AlertOrder,
			AlertSupport: req.AlertSupport,
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

// processUpdate handles an already-acknowledged webhook update. Its context is
// the server's, not the request's: the request context is cancelled the moment
// the response is written, which would abort the work this just promised to do.
func (h *Handler) processUpdate(update tg.Update) {
	ctx, cancel := context.WithTimeout(h.updateCtx, webhookProcessTimeout)
	defer cancel()
	if err := h.updateProcessor.Handle(ctx, update); err != nil {
		log.Printf("telegram webhook update %d: %v", update.UpdateID, err)
	}
}

func (h *Handler) notifyChanged() {
	if h.onSubscriptionsChanged != nil {
		h.onSubscriptionsChanged()
	}
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return authmiddleware.RequireAuth(h.jwtValidator, respondError)(next)
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

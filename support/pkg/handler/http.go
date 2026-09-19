// Package handler is HTTP only: it validates input, calls the service, and
// writes the response. No business rules live here.
package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/authmiddleware"
	"github.com/elug3/dupli1/shared/pkg/settings"
	tg "github.com/elug3/dupli1/shared/pkg/telegram"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

const (
	// maxWebhookBody bounds what Telegram may post. A Bot API update is a few
	// kilobytes; a megabyte is generous and still a bound.
	maxWebhookBody = 1 << 20
	// webhookProcessTimeout bounds the work an acknowledged update may do.
	webhookProcessTimeout = 30 * time.Second
)

// UpdateHandler processes one inbound Telegram update.
type UpdateHandler interface {
	Handle(ctx context.Context, update tg.Update) error
}

// Options are the handler's dependencies.
type Options struct {
	Updates       UpdateHandler
	Inbox         *service.Inbox
	Answers       ports.AnswerRepository
	JWTValidator  authjwt.AccessTokenValidator
	WebhookSecret string
	Settings      settings.Response
	// UpdateContext is the root for work that continues after a webhook has
	// been acknowledged; cancelled on process shutdown.
	UpdateContext context.Context
	HealthProbes  map[string]HealthProbe
}

// HealthProbe reports whether one dependency is reachable.
type HealthProbe func(context.Context) error

type Handler struct {
	updates       UpdateHandler
	inbox         *service.Inbox
	answers       ports.AnswerRepository
	jwtValidator  authjwt.AccessTokenValidator
	webhookSecret string
	settings      settings.Response
	updateCtx     context.Context
	healthProbes  map[string]HealthProbe
}

func New(opts Options) *Handler {
	updateCtx := opts.UpdateContext
	if updateCtx == nil {
		updateCtx = context.Background()
	}
	return &Handler{
		updates:       opts.Updates,
		inbox:         opts.Inbox,
		answers:       opts.Answers,
		jwtValidator:  opts.JWTValidator,
		webhookSecret: opts.WebhookSecret,
		settings:      opts.Settings,
		updateCtx:     updateCtx,
		healthProbes:  opts.HealthProbes,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/support/health", h.health)
	mux.HandleFunc("/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/support/settings", h.settingsHandler)
	mux.HandleFunc("/api/v1/support/telegram/webhook", h.telegramWebhook)

	// Everything below is the manager inbox: authenticated, permission-checked,
	// and the only place a shopper's conversation can be read in full.
	mux.HandleFunc("/api/v1/support/inquiries", h.requireAuth(h.inquiries))
	mux.HandleFunc("/api/v1/support/inquiries/", h.requireAuth(h.inquiryAction))
	mux.HandleFunc("/api/v1/support/answers", h.requireAuth(h.answersHandler))
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return authmiddleware.RequireAuth(h.jwtValidator, respondError)(next)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body := map[string]any{"status": "ok"}
	if len(h.healthProbes) > 0 {
		deps := map[string]string{}
		degraded := false
		for name, probe := range h.healthProbes {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			err := probe(ctx)
			cancel()
			if err != nil {
				deps[name] = "down"
				degraded = true
				continue
			}
			deps[name] = "ok"
		}
		body["dependencies"] = deps
		if degraded {
			body["status"] = "degraded"
		}
	}
	respondJSON(w, http.StatusOK, body)
}

func (h *Handler) settingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	respondJSON(w, http.StatusOK, h.settings)
}

// telegramWebhook receives Bot API updates.
//
// This is the service's one unauthenticated route — it has to be, since
// Telegram calls it. The secret header is what makes it safe, so a missing
// secret fails closed rather than accepting anonymous updates.
func (h *Handler) telegramWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.updates == nil {
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

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}

	var update tg.Update
	if err := json.Unmarshal(body, &update); err != nil {
		respondError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}

	// Acknowledge first, then work. Handling an update means a database write
	// and a Bot API round trip, which can outlast the server's WriteTimeout;
	// a cut-off response has Telegram redeliver an update already in flight.
	go h.processUpdate(update)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) processUpdate(update tg.Update) {
	ctx, cancel := context.WithTimeout(h.updateCtx, webhookProcessTimeout)
	defer cancel()

	if err := h.updates.Handle(ctx, update); err != nil {
		// Deliberately no update body in the log line: a shopper's message is
		// customer data and does not belong in CloudWatch.
		log.Printf("support telegram update %d: %v", update.UpdateID, err)
	}
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/livefeed"
	"github.com/elug3/dupli1/support/pkg/service"
)

const (
	// streamHeartbeat keeps proxies (nginx's 60s read timeout, the BFFs,
	// Cloudflare) from closing an idle stream.
	streamHeartbeat = 20 * time.Second
	// streamMaxLifetime bounds a stream whose token carries no expiry.
	streamMaxLifetime = 15 * time.Minute
	// streamWriteTimeout bounds each write, so a client that stops reading
	// cannot hold the handler once the server-wide WriteTimeout is lifted.
	streamWriteTimeout = 10 * time.Second
	// streamRetryMs is the browser's reconnect delay after a stream ends.
	streamRetryMs = 3000
	// maxWebBody bounds a chat send. 2000 characters of Hangul is 6 KB.
	maxWebBody = 64 << 10
)

// webMessageJSON is a transcript line as the shopper sees it. No author and
// no delivery error: which manager answered is staff business, and the
// shopper is told only that Dupli1 replied.
type webMessageJSON struct {
	ID        string          `json:"id"`
	Direction string          `json:"direction"`
	Kind      string          `json:"kind"`
	Body      string          `json:"body"`
	Ref       json.RawMessage `json:"ref,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type webInquiryJSON struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	ProductID string    `json:"product_id,omitempty"`
	SkuID     string    `json:"sku_id,omitempty"`
	OrderID   string    `json:"order_id,omitempty"`
	OpenedAt  time.Time `json:"opened_at"`
}

type webConversationJSON struct {
	Inquiry       *webInquiryJSON  `json:"inquiry"`
	Messages      []webMessageJSON `json:"messages"`
	Unread        int              `json:"unread"`
	ServiceOpen   bool             `json:"service_open"`
	ServiceWindow string           `json:"service_window"`
}

func webConversationToJSON(view *service.WebConversationView) webConversationJSON {
	out := webConversationJSON{
		Messages:      make([]webMessageJSON, 0, len(view.Messages)),
		Unread:        view.Unread,
		ServiceOpen:   view.ServiceOpen,
		ServiceWindow: view.ServiceWindow,
	}
	if view.Inquiry != nil {
		out.Inquiry = &webInquiryJSON{
			ID:        view.Inquiry.ID,
			Status:    view.Inquiry.Status,
			ProductID: view.Inquiry.ProductID,
			SkuID:     view.Inquiry.SkuID,
			OrderID:   view.Inquiry.OrderID,
			OpenedAt:  view.Inquiry.OpenedAt,
		}
	}
	for _, message := range view.Messages {
		out.Messages = append(out.Messages, webMessageJSON{
			ID:        message.ID,
			Direction: message.Direction,
			Kind:      kindOf(message),
			Body:      message.Body,
			Ref:       refOf(message),
			CreatedAt: message.CreatedAt,
		})
	}
	return out
}

func kindOf(message domain.Message) string {
	if message.Kind == "" {
		return domain.MessageText
	}
	return message.Kind
}

// refOf returns a reference card's snapshot, or nothing once the retention
// purge has replaced the message's words: the card then goes with them.
func refOf(message domain.Message) json.RawMessage {
	if len(message.RefSnapshot) == 0 || message.Body == domain.PurgedBody || !json.Valid(message.RefSnapshot) {
		return nil
	}
	return json.RawMessage(message.RefSnapshot)
}

// customerFrom reads the shopper off the token. Service accounts have no
// conversation to hold; customers and managers (who shop too) do.
func customerFrom(r *http.Request) (service.Customer, bool) {
	claims, ok := authjwt.FromContext(r.Context())
	if !ok || strings.TrimSpace(claims.UserID) == "" || claims.AccountType == "service" {
		return service.Customer{}, false
	}
	return service.Customer{ID: claims.UserID, Email: claims.Email, Bearer: bearerOf(r)}, true
}

func bearerOf(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if len(header) > 7 && strings.EqualFold(header[:7], "Bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func (h *Handler) webConversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	customer, ok := customerFrom(r)
	if !ok {
		respondError(w, http.StatusForbidden, "a customer account is required")
		return
	}
	view, err := h.webChat.Conversation(r.Context(), customer)
	h.respondWeb(w, view, err)
}

func (h *Handler) webMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	customer, ok := customerFrom(r)
	if !ok {
		respondError(w, http.StatusForbidden, "a customer account is required")
		return
	}
	var req struct {
		Body      string `json:"body"`
		ProductID string `json:"product_id"`
		SkuID     string `json:"sku_id"`
		OrderID   string `json:"order_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWebBody)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	view, err := h.webChat.Send(r.Context(), customer, service.SendInput{
		Body: req.Body, ProductID: req.ProductID, SkuID: req.SkuID, OrderID: req.OrderID,
	})
	h.respondWeb(w, view, err)
}

func (h *Handler) webRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	customer, ok := customerFrom(r)
	if !ok {
		respondError(w, http.StatusForbidden, "a customer account is required")
		return
	}
	if err := h.webChat.MarkRead(r.Context(), customer); err != nil {
		respondWebError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) webClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	customer, ok := customerFrom(r)
	if !ok {
		respondError(w, http.StatusForbidden, "a customer account is required")
		return
	}
	view, err := h.webChat.Close(r.Context(), customer)
	h.respondWeb(w, view, err)
}

func (h *Handler) respondWeb(w http.ResponseWriter, view *service.WebConversationView, err error) {
	if err != nil {
		respondWebError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"conversation": webConversationToJSON(view)})
}

// respondWebError maps chat errors to statuses. Each has a stable `code` the
// storefront switches on, so its Korean copy is not tied to these strings.
func respondWebError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidMessage):
		respondCode(w, http.StatusBadRequest, "invalid_message", err.Error())
	case errors.Is(err, service.ErrInvalidReference):
		respondCode(w, http.StatusUnprocessableEntity, "invalid_reference", err.Error())
	case errors.Is(err, service.ErrReferencesUnavailable):
		respondCode(w, http.StatusServiceUnavailable, "reference_unavailable", service.ErrReferencesUnavailable.Error())
	case errors.Is(err, service.ErrRateLimited):
		w.Header().Set("Retry-After", "60")
		respondCode(w, http.StatusTooManyRequests, "rate_limited", err.Error())
	case errors.Is(err, service.ErrCustomerRequired):
		respondCode(w, http.StatusForbidden, "customer_required", err.Error())
	default:
		// The message is deliberately generic: the error may name a table or
		// a query, and the shopper's words must never reach a log line.
		log.Printf("support web chat: %v", err)
		respondCode(w, http.StatusInternalServerError, "internal", "could not complete the request")
	}
}

func respondCode(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, map[string]string{"error": message, "code": code})
}

// webEvents streams the shopper's own consultation changes.
func (h *Handler) webEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	customer, ok := customerFrom(r)
	if !ok {
		respondError(w, http.StatusForbidden, "a customer account is required")
		return
	}
	if h.hub == nil {
		respondError(w, http.StatusServiceUnavailable, "chat stream not configured")
		return
	}
	sub := h.hub.SubscribeCustomer(customer.ID)
	h.stream(w, r, sub)
}

// inboxEvents streams every consultation change to staff.
func (h *Handler) inboxEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	if !canRead(claims) {
		respondError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if h.hub == nil {
		respondError(w, http.StatusServiceUnavailable, "inbox stream not configured")
		return
	}
	sub := h.hub.SubscribeInbox()
	h.stream(w, r, sub)
}

// stream writes a subscription as Server-Sent Events until the client leaves,
// falls behind, or the token that opened it expires.
//
// The first frame is `event: ready`. Clients load (or reload) through REST on
// it, which is what makes a reconnect lossless without a replay buffer: every
// change made while the stream was down is already in the REST answer.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request, sub *livefeed.Subscription) {
	defer h.hub.Unsubscribe(sub)

	rc := http.NewResponseController(w)
	// The server-wide WriteTimeout would cut every stream; each write below
	// sets its own deadline instead.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	// nginx honours this per response: without it the gateway buffers the
	// stream and nothing arrives until the buffer fills.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(format string, args ...any) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("retry: %d\n\nevent: ready\ndata: {}\n\n", streamRetryMs) {
		return
	}

	lifetime := streamMaxLifetime
	if claims, ok := authjwt.FromContext(r.Context()); ok && !claims.ExpiresAt.IsZero() {
		if until := time.Until(claims.ExpiresAt); until < lifetime {
			lifetime = until
		}
	}
	expired := time.NewTimer(lifetime)
	defer expired.Stop()
	heartbeat := time.NewTicker(streamHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.updateCtx.Done():
			// Shutting down: Shutdown waits for handlers, and a stream would
			// otherwise hold it for the whole grace period.
			return
		case <-expired.C:
			// Token ran out: end cleanly; the client reconnects through its
			// BFF, which attaches a fresh token.
			return
		case <-heartbeat.C:
			if !write(": ping\n\n") {
				return
			}
		case frame, ok := <-sub.C:
			if !ok {
				return
			}
			if !write("event: %s\ndata: %s\n\n", frame.Event, frame.Data) {
				return
			}
		}
	}
}

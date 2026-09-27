package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/elug3/dupli1/order/pkg/livefeed"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

const (
	// streamHeartbeat keeps proxies (nginx's 60s read timeout, the manage-web
	// BFF, Cloudflare) from closing an idle stream.
	streamHeartbeat = 20 * time.Second
	// streamMaxLifetime bounds a stream whose token carries no expiry.
	streamMaxLifetime = 15 * time.Minute
	// streamWriteTimeout bounds each write, so a client that stops reading
	// cannot hold the handler forever now that the server-wide
	// WriteTimeout is lifted for this response.
	streamWriteTimeout = 10 * time.Second
	// streamRetryMs is the browser's reconnect delay after a stream ends.
	streamRetryMs = 3000
)

// WithOrderFeed enables GET /api/v1/orders/events.
func (h *Handler) WithOrderFeed(hub *livefeed.Hub) *Handler {
	h.feed = hub
	return h
}

// orderEvents streams order changes to the admin console as Server-Sent
// Events (docs/order-live-events.md). Requires order.read.all, like listing
// every order.
//
// The token is checked once, when the stream opens, so the stream ends when
// that token expires. The browser reconnects on its own with Last-Event-ID,
// through the manage-web BFF, which attaches a fresh token — so a revoked
// permission stops the stream within one access-token lifetime, the same
// window every other API has.
func (h *Handler) orderEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	if h.jwtValidator != nil && !permissions.BypassesOrderReadABAC(claims.Permissions) {
		respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
		return
	}
	if h.feed == nil {
		respondError(w, http.StatusServiceUnavailable, "order event stream not configured")
		return
	}

	rc := http.NewResponseController(w)
	// The server-wide WriteTimeout (25s) would cut every stream; each write
	// below sets its own deadline instead.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	sub, replay, reset := h.feed.Subscribe(r.Header.Get("Last-Event-ID"))
	defer h.feed.Unsubscribe(sub)

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	// nginx honours this per response: without it the gateway buffers the
	// stream and the console sees nothing until the buffer fills.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(format string, args ...any) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout))
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	writeFrame := func(f livefeed.Frame) bool {
		return write("id: %s\nevent: order\ndata: %s\n\n", f.ID, f.Data)
	}

	if !write("retry: %d\n\n", streamRetryMs) {
		return
	}
	if reset && !write("event: reset\ndata: {\"reason\":\"gap\"}\n\n") {
		return
	}
	for _, f := range replay {
		if !writeFrame(f) {
			return
		}
	}

	lifetime := streamMaxLifetime
	if !claims.ExpiresAt.IsZero() {
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
		case <-expired.C:
			// Token ran out: end cleanly; the browser reconnects with its
			// cursor and the BFF's fresh token.
			return
		case <-heartbeat.C:
			if !write(": ping\n\n") {
				return
			}
		case f, ok := <-sub.C:
			if !ok {
				// Dropped for falling behind; the reconnect replays.
				return
			}
			if !writeFrame(f) {
				return
			}
		}
	}
}

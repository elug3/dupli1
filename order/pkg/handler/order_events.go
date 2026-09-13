package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

const (
	// orderEventHeartbeat keeps idle connections alive through proxies; the ALB
	// in front of the admin app idles connections out at 60s by default.
	orderEventHeartbeat = 20 * time.Second
	// orderEventMaxAge bounds one connection's life. The access token is only
	// validated when the stream opens, so expiring the stream is what forces
	// the caller's token and permissions to be checked again — EventSource
	// reconnects on its own, and an unchanged cursor means no data is resent.
	orderEventMaxAge = 15 * time.Minute
	// orderEventRetry is the reconnect delay advertised to the browser.
	orderEventRetry = 3 * time.Second
)

// orderEvents streams order changes to admin clients as Server-Sent Events.
//
// GET /api/v1/orders/events (requires order.read.all, like the all-orders list)
func (h *Handler) orderEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, _ := authjwt.FromContext(r.Context())
	// Same gate as GET /api/v1/orders without customer_id: this is the
	// all-orders feed, so it is admin-only. There is no per-customer stream.
	if h.jwtValidator != nil && !permissions.BypassesOrderReadABAC(claims.Permissions) {
		respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
		return
	}
	if h.orderStream == nil {
		// No NATS configured: order changes never reach this task, so a stream
		// would be silently empty. Say so instead.
		respondError(w, http.StatusServiceUnavailable, "live order events not configured")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	// no-cache stops an intermediary replaying a stale stream; no-transform asks
	// any gzip layer to leave it alone. Neither is load-bearing in today's chain
	// (the gateway runs with gzip off, and the admin BFF's express compression is
	// flushed per chunk by the React Router node adapter), but gzipping 200-byte
	// frames is pure overhead, and a gzip layer added later would quietly buffer
	// the stream rather than fail loudly.
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("Connection", "keep-alive")
	// Turns nginx proxy buffering off for this response whatever the location
	// block says, so the feed still streams through a gateway nobody tuned.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sub, replay, gapped := h.orderStream.Subscribe(r.Header.Get("Last-Event-ID"))
	defer sub.Close()

	if _, err := fmt.Fprintf(w, "retry: %d\n\n", orderEventRetry.Milliseconds()); err != nil {
		return
	}
	if gapped {
		// Cannot prove the client saw everything since its cursor.
		if err := writeSSEEvent(w, "", "reset", []byte(`{"reason":"gap"}`)); err != nil {
			return
		}
	}
	for _, ev := range replay {
		if err := writeSSEEvent(w, ev.ID, ev.Type, ev.Data); err != nil {
			return
		}
	}
	flusher.Flush()

	heartbeat := time.NewTicker(orderEventHeartbeat)
	defer heartbeat.Stop()
	maxAge := time.NewTimer(orderEventMaxAge)
	defer maxAge.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-maxAge.C:
			return
		case <-heartbeat.C:
			// Comment frame: EventSource ignores it, proxies see traffic.
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-sub.C():
			if !ok {
				// Hub dropped this client for lagging; it must reload from REST.
				_ = writeSSEEvent(w, "", "reset", []byte(`{"reason":"lagged"}`))
				flusher.Flush()
				return
			}
			if err := writeSSEEvent(w, ev.ID, ev.Type, ev.Data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes one event frame. data must be free of literal newlines,
// which holds for JSON (it escapes them inside strings).
func writeSSEEvent(w io.Writer, id, eventType string, data []byte) error {
	var b strings.Builder
	if id != "" {
		b.WriteString("id: ")
		b.WriteString(id)
		b.WriteString("\n")
	}
	if eventType != "" {
		b.WriteString("event: ")
		b.WriteString(eventType)
		b.WriteString("\n")
	}
	b.WriteString("data: ")
	b.Write(data)
	b.WriteString("\n\n")
	_, err := io.WriteString(w, b.String())
	return err
}

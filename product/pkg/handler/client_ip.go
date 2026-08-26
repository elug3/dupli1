package handler

import (
	"net"
	"net/http"
	"strings"
)

// clientIP extracts the visitor's IP for view-count dedup purposes.
// Preference order: first hop of X-Forwarded-For -> X-Real-IP -> RemoteAddr host.
// Trusts the nginx gateway (api/nginx.conf) to set X-Real-IP/X-Forwarded-For on
// the /api/v1/products location; never returns an empty string.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xrip       string
		remoteAddr string
		want       string
	}{
		{
			name: "X-Forwarded-For single value",
			xff:  "1.2.3.4",
			want: "1.2.3.4",
		},
		{
			name: "X-Forwarded-For multi value takes first, trimmed",
			xff:  "1.2.3.4, 5.6.7.8",
			want: "1.2.3.4",
		},
		{
			name: "X-Forwarded-For empty falls back to X-Real-IP",
			xrip: "9.9.9.9",
			want: "9.9.9.9",
		},
		{
			name:       "no headers falls back to RemoteAddr host",
			remoteAddr: "192.0.2.1:1234",
			want:       "192.0.2.1",
		},
		{
			name:       "malformed RemoteAddr (no port) falls back to raw value",
			remoteAddr: "192.0.2.1",
			want:       "192.0.2.1",
		},
		{
			name:       "X-Forwarded-For blank after trim falls through",
			xff:        " , ",
			remoteAddr: "192.0.2.1:1234",
			want:       "192.0.2.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil)
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xrip != "" {
				r.Header.Set("X-Real-IP", tc.xrip)
			}
			if tc.remoteAddr != "" {
				r.RemoteAddr = tc.remoteAddr
			}
			if got := clientIP(r); got != tc.want {
				t.Fatalf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

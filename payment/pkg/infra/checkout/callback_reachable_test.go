package checkout

import "testing"

// receiveUrl is built from PublicBaseURL and NANO POSTs the approval to it from
// its own servers. A base that only resolves on this machine means the callback
// never arrives and payments strand at requires_payment with nothing logged —
// which is exactly what happened before this check existed.
func TestCallbackReachable(t *testing.T) {
	cases := []struct {
		base string
		want bool
		why  string
	}{
		{"https://pay.dupli1.com", true, "public host"},
		{"https://abc123.trycloudflare.com", true, "tunnel host"},
		{"http://203.0.113.10:8080", true, "public IP"},

		{"", false, "unset falls back to localhost"},
		{"http://localhost:8080", false, "loopback name"},
		{"http://LOCALHOST:8080", false, "loopback name, any case"},
		{"http://api.localhost", false, ".localhost suffix"},
		{"http://127.0.0.1:8080", false, "loopback IPv4"},
		{"http://[::1]:8080", false, "loopback IPv6"},
		{"http://0.0.0.0:8080", false, "unspecified address"},
		{"http://192.168.1.20:8080", false, "private LAN"},
		{"http://10.0.0.5:8080", false, "private LAN"},
		{"http://172.16.4.4:8080", false, "private LAN"},
		{"http://169.254.10.1", false, "link-local"},
		{"://nonsense", false, "unparseable"},
	}
	for _, tc := range cases {
		got := NanoConfig{PublicBaseURL: tc.base}.CallbackReachable()
		if got != tc.want {
			t.Errorf("CallbackReachable(%q) = %t, want %t (%s)", tc.base, got, tc.want, tc.why)
		}
	}
}

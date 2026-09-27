package httpauth_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
)

const testAPIKey = "dk_test_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func tokenExpiringIn(d time.Duration, n int32) string {
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(d).Unix(), "n": n})
	return "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"
}

// exchangeServer answers POST /api/v1/auth/token like auth, minting tokens
// that expire after ttl, and counts exchanges.
func exchangeServer(t *testing.T, ttl time.Duration) (*httptest.Server, *atomic.Int32) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/token" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "ApiKey "+testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_api_key"}`))
			return
		}
		c := n.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": tokenExpiringIn(ttl, c), "token_type": "Bearer", "expires_in": int(ttl.Seconds())})
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestAPIKeyTokenSource_CachesUntilNearExpiry(t *testing.T) {
	srv, n := exchangeServer(t, 15*time.Minute)
	src := httpauth.NewAPIKeyTokenSource(srv.URL+"/", testAPIKey, srv.Client())

	first, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	again, _ := src.Token(t.Context())
	if again != first || n.Load() != 1 {
		t.Fatalf("second call exchanged again (%d exchanges)", n.Load())
	}

	src.Invalidate() // what the stock/payment clients do after a 401
	if tok, _ := src.Token(t.Context()); tok == first || n.Load() != 2 {
		t.Fatalf("Invalidate did not force an exchange (%d exchanges)", n.Load())
	}
}

func TestAPIKeyTokenSource_ExchangesWithinSkew(t *testing.T) {
	srv, n := exchangeServer(t, 30*time.Second) // inside the 60s skew
	src := httpauth.NewAPIKeyTokenSource(srv.URL, testAPIKey, srv.Client())
	_, _ = src.Token(t.Context())
	_, _ = src.Token(t.Context())
	if n.Load() != 2 {
		t.Fatalf("a token about to expire was reused (%d exchanges)", n.Load())
	}
}

func TestAPIKeyTokenSource_ErrorNeverCarriesTheKey(t *testing.T) {
	srv, _ := exchangeServer(t, time.Minute)
	wrong := "dk_test_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	_, err := httpauth.NewAPIKeyTokenSource(srv.URL, wrong, srv.Client()).Token(t.Context())
	if err == nil || !strings.Contains(err.Error(), "invalid_api_key") {
		t.Fatalf("err = %v, want the auth error", err)
	}
	if strings.Contains(fmt.Sprint(err), wrong) || strings.Contains(fmt.Sprint(err), "BBBB") {
		t.Fatalf("error leaks the key: %v", err)
	}
}

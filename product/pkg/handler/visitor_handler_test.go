package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/infra/ratelimit"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

const browserUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 Safari/604.1"

func newVisitorMux() *http.ServeMux {
	store := memory.NewProductStore()
	store.Catalog = memory.NewCatalogStore()
	h := handler.NewHandler(service.NewProductSearchService(store, nil), service.NewPromotionService(memory.NewPromotionStore()), nil, service.NewCatalogService(store.Catalog)).
		WithVisitorService(service.NewVisitorService(memory.NewVisitorStore()))
	validator := authjwt.NewHMACValidator(accessControlSecret)
	mux := http.NewServeMux()
	// Mirrors bootstrap.
	mux.HandleFunc("POST "+handler.RouteVisits, h.RecordVisit)
	mux.Handle("GET "+handler.RouteVisitorsReport, middleware.RequireAuth(validator, middleware.RequireAnyPermission(permissions.ProductRead)(http.HandlerFunc(h.VisitorReport))))
	return mux
}

func postVisit(t *testing.T, mux *http.ServeMux, ua string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, handler.RouteVisits, nil)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("POST visits: status=%d, want 204", w.Code)
	}
	return w
}

func visitorReport(t *testing.T, mux *http.ServeMux) domain.VisitorReport {
	t.Helper()
	tok := makeAccessToken(t, "mgr", []string{permissions.ProductRead})
	w := serve(t, mux, http.MethodGet, handler.RouteVisitorsReport+"?granularity=week", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET report: status=%d body=%s", w.Code, w.Body.String())
	}
	var report domain.VisitorReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return report
}

func TestRecordVisit_MintsCookieAndCountsBrowserOncePerDay(t *testing.T) {
	mux := newVisitorMux()

	w := postVisit(t, mux, browserUA, nil)
	var guest *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "dupli1_guest" {
			guest = c
		}
	}
	if guest == nil || !handler.ValidGuestID(guest.Value) || !guest.HttpOnly {
		t.Fatalf("want an HttpOnly dupli1_guest cookie, got %+v", w.Result().Cookies())
	}

	again := postVisit(t, mux, browserUA, guest)
	if len(again.Result().Cookies()) != 0 {
		t.Fatal("a browser that sent its cookie should not be given another")
	}
	postVisit(t, mux, browserUA, nil) // a second browser

	if got := visitorReport(t, mux).Today.UniqueVisitors; got != 2 {
		t.Fatalf("today's unique visitors = %d, want 2", got)
	}
}

func TestRecordVisit_IgnoresBotsAndPrefetch(t *testing.T) {
	mux := newVisitorMux()

	postVisit(t, mux, "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", nil)
	postVisit(t, mux, "", nil)

	req := httptest.NewRequest(http.MethodPost, handler.RouteVisits, nil)
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Sec-Purpose", "prefetch")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || len(w.Result().Cookies()) != 0 {
		t.Fatalf("prefetch: status=%d cookies=%v", w.Code, w.Result().Cookies())
	}

	if got := visitorReport(t, mux).Today.UniqueVisitors; got != 0 {
		t.Fatalf("today's unique visitors = %d, want 0", got)
	}
}

func TestRecordVisit_RateLimitCapsPerIP(t *testing.T) {
	store := memory.NewProductStore()
	store.Catalog = memory.NewCatalogStore()
	h := handler.NewHandler(service.NewProductSearchService(store, nil), service.NewPromotionService(memory.NewPromotionStore()), nil, service.NewCatalogService(store.Catalog)).
		WithVisitorService(service.NewVisitorService(memory.NewVisitorStore()))
	// Same wiring as bootstrap.newVisitRateLimiter, but a tiny budget so the
	// test finishes quickly.
	limiter := ratelimit.New(ratelimit.NewMemoryCounter(), 2, time.Minute)
	mux := http.NewServeMux()
	mux.Handle("POST "+handler.RouteVisits, limiter.Middleware(nil)(http.HandlerFunc(h.RecordVisit)))

	const ip = "203.0.113.55:4242"
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, handler.RouteVisits, nil)
		req.Header.Set("User-Agent", browserUA)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("visit %d: status=%d, want 204", i, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, handler.RouteVisits, nil)
	req.Header.Set("User-Agent", browserUA)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("over cap: status=%d, want 429; body=%s", w.Code, w.Body.String())
	}
}

func TestVisitorReport_RequiresProductRead(t *testing.T) {
	mux := newVisitorMux()

	if w := serve(t, mux, http.MethodGet, handler.RouteVisitorsReport, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status=%d, want 401", w.Code)
	}
	customer := makeAccessToken(t, "cust", nil)
	if w := serve(t, mux, http.MethodGet, handler.RouteVisitorsReport, customer, nil); w.Code != http.StatusForbidden {
		t.Fatalf("customer: status=%d, want 403", w.Code)
	}
	mgr := makeAccessToken(t, "mgr", []string{permissions.ProductRead})
	if w := serve(t, mux, http.MethodGet, handler.RouteVisitorsReport+"?granularity=day", mgr, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad granularity: status=%d, want 400", w.Code)
	}
}

package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/handler"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func putVideo(t *testing.T, mux *http.ServeMux, orderID, token, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/orders/"+orderID+"/inspection-video", strings.NewReader(body))
	req.Header.Set("Authorization", bearerHeader(token))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func getVideo(t *testing.T, mux *http.ServeMux, orderID, token, rangeHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID+"/inspection-video", nil)
	req.Header.Set("Authorization", bearerHeader(token))
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestInspectionVideoHTTP(t *testing.T) {
	repo := memory.NewRepository()
	svc := service.New(repo, &fakeStock{}).WithProduct(&fakeProduct{price: 1000}).
		WithInspectionVideos(memory.NewVideoStore(), 0)
	mux := newMux(handler.New(svc, authjwt.NewHMACValidator(testSecret)))

	orderID := seedPaidOrder(t, svc, "u-1")
	if _, err := svc.ConfirmOrder(t.Context(), orderID); err != nil {
		t.Fatalf("ConfirmOrder: %v", err)
	}
	staff := makeToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}))
	customer := makeToken(t, "u-1", nil)
	stranger := makeToken(t, "u-2", nil)
	shipBody := map[string]string{"carrier": "cj", "tracking_number": "123456789012"}

	// Ship without a video: 409 with a reason the console can branch on.
	w := do(t, mux, http.MethodPost, "/api/v1/orders/"+orderID+"/ship", staff, shipBody)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"reason":"inspection_video_required"`) {
		t.Fatalf("ship without video: %d %s", w.Code, w.Body.String())
	}

	if w := putVideo(t, mux, orderID, customer, "video/mp4", "film"); w.Code != http.StatusForbidden {
		t.Fatalf("customer upload: status = %d, want 403", w.Code)
	}
	if w := putVideo(t, mux, orderID, staff, "image/png", "png"); w.Code != http.StatusBadRequest {
		t.Fatalf("image upload: status = %d, want 400; %s", w.Code, w.Body.String())
	}

	w = putVideo(t, mux, orderID, staff, "video/mp4", "0123456789")
	if w.Code != http.StatusOK {
		t.Fatalf("upload: status = %d; %s", w.Code, w.Body.String())
	}
	var uploaded map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	video, _ := uploaded["inspection_video"].(map[string]any)
	if video["content_type"] != "video/mp4" || video["size_bytes"] != float64(10) || video["uploaded_by"] != "mgr-1" {
		t.Fatalf("inspection_video = %v", uploaded["inspection_video"])
	}
	if _, leaked := video["object_key"]; leaked {
		t.Fatal("the storage key must not be in the order JSON")
	}

	// Before shipping only staff may watch.
	if w := getVideo(t, mux, orderID, customer, ""); w.Code != http.StatusNotFound {
		t.Fatalf("customer before ship: status = %d, want 404", w.Code)
	}
	if w := getVideo(t, mux, orderID, staff, ""); w.Code != http.StatusOK || w.Body.String() != "0123456789" {
		t.Fatalf("staff before ship: %d %q", w.Code, w.Body.String())
	}

	if w := do(t, mux, http.MethodPost, "/api/v1/orders/"+orderID+"/ship", staff, shipBody); w.Code != http.StatusOK {
		t.Fatalf("ship with video: %d %s", w.Code, w.Body.String())
	}

	w = getVideo(t, mux, orderID, customer, "bytes=2-5")
	if w.Code != http.StatusPartialContent {
		t.Fatalf("customer range: status = %d, want 206; %s", w.Code, w.Body.String())
	}
	if body, _ := io.ReadAll(w.Body); string(body) != "2345" {
		t.Fatalf("range body = %q, want 2345", body)
	}
	if got := w.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if w := getVideo(t, mux, orderID, stranger, ""); w.Code != http.StatusForbidden {
		t.Fatalf("another customer: status = %d, want 403", w.Code)
	}

	// Fulfilled: replacing is refused and the order says so.
	if _, err := svc.DeliverOrder(t.Context(), orderID, "driver-1"); err != nil {
		t.Fatalf("DeliverOrder: %v", err)
	}
	if _, err := svc.ConfirmReceipt(t.Context(), orderID); err != nil {
		t.Fatalf("ConfirmReceipt: %v", err)
	}
	w = putVideo(t, mux, orderID, staff, "video/mp4", "late")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"reason":"inspection_video_locked"`) {
		t.Fatalf("upload after receipt: %d %s", w.Code, w.Body.String())
	}
	got, err := svc.GetOrder(t.Context(), orderID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if !got.InspectionVideoLocked || got.Status != domain.StatusFulfilled {
		t.Fatalf("locked = %v, status = %s", got.InspectionVideoLocked, got.Status)
	}
}

func TestInspectionVideoUploadWithoutStorageIs503(t *testing.T) {
	h, svc := newTestHandler(t)
	svc.WithInspectionVideos(nil, 0)
	mux := newMux(h)
	orderID := seedPaidOrder(t, svc, "u-1")
	staff := makeToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleOrderManager}))
	if w := putVideo(t, mux, orderID, staff, "video/mp4", "film"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; %s", w.Code, w.Body.String())
	}
}

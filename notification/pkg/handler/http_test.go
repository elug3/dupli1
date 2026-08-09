package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/handler"
	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "notification-handler-test"

func makeToken(t *testing.T, userID string, perms []string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         userID,
		"type":        "access",
		"permissions": perms,
		"exp":         time.Now().Add(time.Hour).Unix(),
		"iat":         time.Now().Unix(),
	})
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return signed
}

func bearer(token string) string { return "Bearer " + token }

func newMux(h *handler.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func newTestHandler(t *testing.T, webhookSecret string) (*handler.Handler, *service.TelegramSubscriptions) {
	t.Helper()
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	h := handler.New(handler.Options{
		TelegramSubs:    subs,
		UpdateProcessor: &telegram.UpdateProcessor{},
		WebhookSecret:   webhookSecret,
		JWTValidator:    authjwt.NewHMACValidator(testSecret),
		Settings:        settings.Response{Service: "notification"},
	})
	return h, subs
}

func doJSON(t *testing.T, mux http.Handler, method, path, auth string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestTelegramWebhookFailsClosedWithoutSecret(t *testing.T) {
	h, _ := newTestHandler(t, "")
	mux := newMux(h)

	rec := doJSON(t, mux, http.MethodPost, "/api/v1/notification/telegram/webhook", "", map[string]any{"update_id": 1})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
}

func TestTelegramWebhookRejectsInvalidSecret(t *testing.T) {
	h, _ := newTestHandler(t, "expected-secret")
	mux := newMux(h)

	rec := doJSON(t, mux, http.MethodPost, "/api/v1/notification/telegram/webhook", "", map[string]any{"update_id": 1})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notification/telegram/webhook", bytes.NewReader([]byte(`{"update_id":1}`)))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "expected-secret")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestTelegramWebhookRejectsInvalidJSON(t *testing.T) {
	h, _ := newTestHandler(t, "expected-secret")
	mux := newMux(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notification/telegram/webhook", bytes.NewReader([]byte(`not-json`)))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "expected-secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestTelegramSubscriptionsRequireAuth(t *testing.T) {
	h, _ := newTestHandler(t, "")
	mux := newMux(h)

	rec := doJSON(t, mux, http.MethodGet, "/api/v1/notification/telegram/subscriptions", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}
}

func TestTelegramSubscriptionsFailClosedWithoutValidator(t *testing.T) {
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	h := handler.New(handler.Options{
		TelegramSubs: subs,
		JWTValidator: nil,
		Settings:     settings.Response{Service: "notification"},
	})
	mux := newMux(h)

	rec := doJSON(t, mux, http.MethodGet, "/api/v1/notification/telegram/subscriptions", bearer(makeToken(t, "mgr-1", []string{permissions.NotificationTelegramRead})), nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
}

func TestTelegramSubscriptionsReadVsManagePermissions(t *testing.T) {
	h, subs := newTestHandler(t, "")
	mux := newMux(h)
	ctx := context.Background()

	readToken := makeToken(t, "reader-1", []string{permissions.NotificationTelegramRead})
	manageToken := makeToken(t, "manager-1", []string{permissions.NotificationTelegramManage})

	rec := doJSON(t, mux, http.MethodPost, "/api/v1/notification/telegram/subscriptions", bearer(readToken), map[string]any{
		"chat_id": "-100555", "chat_label": "Ops", "alert_order": true,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader create status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodPost, "/api/v1/notification/telegram/subscriptions", bearer(manageToken), map[string]any{
		"chat_id": "-100555", "chat_label": "Ops", "alert_order": true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("manager create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}

	pending, err := subs.RegisterFromMessage(ctx, ports.TelegramSubscriptionInput{
		TelegramUserID: int64Ptr(99),
		ChatID:         "99",
		ChatType:       "private",
		ChatLabel:      "Pending User",
	})
	if err != nil {
		t.Fatalf("register pending: %v", err)
	}

	rec = doJSON(t, mux, http.MethodGet, "/api/v1/notification/telegram/subscriptions?status=pending", bearer(readToken), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var listBody struct {
		Items []domain.TelegramSubscription `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Items) == 0 {
		t.Fatal("expected at least one pending subscription")
	}

	rec = doJSON(t, mux, http.MethodPost, fmt.Sprintf("/api/v1/notification/telegram/subscriptions/%s/accept", pending.ID), bearer(readToken), map[string]any{
		"alert_order": true,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader accept status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, http.MethodPost, fmt.Sprintf("/api/v1/notification/telegram/subscriptions/%s/accept", pending.ID), bearer(manageToken), map[string]any{
		"alert_order":   true,
		"alert_product": false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("manager accept status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var accepted domain.TelegramSubscription
	if err := json.NewDecoder(rec.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Status != domain.SubscriptionStatusAccepted || !accepted.AlertOrder {
		t.Fatalf("accepted subscription: %+v", accepted)
	}
}

func TestTelegramSubscriptionRejectAndDelete(t *testing.T) {
	h, subs := newTestHandler(t, "")
	mux := newMux(h)
	manageToken := makeToken(t, "manager-1", []string{permissions.NotificationTelegramManage})

	pending, err := subs.RegisterFromMessage(context.Background(), ports.TelegramSubscriptionInput{
		TelegramUserID: int64Ptr(77),
		ChatID:         "77",
		ChatType:       "private",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := doJSON(t, mux, http.MethodPost, fmt.Sprintf("/api/v1/notification/telegram/subscriptions/%s/reject", pending.ID), bearer(manageToken), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var rejected domain.TelegramSubscription
	if err := json.NewDecoder(rec.Body).Decode(&rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Status != domain.SubscriptionStatusRejected {
		t.Fatalf("status = %q, want rejected", rejected.Status)
	}

	manual, err := subs.CreateManual(context.Background(), ports.TelegramManualInput{
		ChatID:     "-100888",
		ChatLabel:  "Delete me",
		AlertOrder: true,
		AcceptedBy: "manager-1",
	})
	if err != nil {
		t.Fatalf("create manual: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/notification/telegram/subscriptions/%s", manual.ID), nil)
	req.Header.Set("Authorization", bearer(manageToken))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
}

func int64Ptr(v int64) *int64 { return &v }

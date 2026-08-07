package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/handler"
	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
)

func TestTelegramWebhookRejectsMissingSecret(t *testing.T) {
	h := handler.New(handler.Options{
		UpdateProcessor: &telegram.UpdateProcessor{},
		WebhookSecret:   "",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notification/telegram/webhook", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestTelegramWebhookRejectsInvalidSecret(t *testing.T) {
	h := handler.New(handler.Options{
		UpdateProcessor: &telegram.UpdateProcessor{},
		WebhookSecret:   "expected-secret",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notification/telegram/webhook", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret")
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

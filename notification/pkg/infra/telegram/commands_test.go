package telegram_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/service"
)

type stubLookup struct {
	sub *domain.TelegramSubscription
}

func (s *stubLookup) RegisterFromMessage(ctx context.Context, in telegram.SubscriptionInput) (*domain.TelegramSubscription, error) {
	return s.sub, nil
}

func (s *stubLookup) FindForMessage(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error) {
	return s.sub, nil
}

func TestIsStartCommand(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"/start", true},
		{"/start payload", true},
		{"/start@MHYM7_BOT", true},
		{"/start@MHYM7_BOT hello", true},
		{"/help", false},
		{"start", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := telegram.IsStartCommand(tc.text); got != tc.want {
			t.Fatalf("IsStartCommand(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestFormatStartReplyIncludesChatID(t *testing.T) {
	reply := telegram.FormatStartReply(telegram.Chat{
		ID:        -1001234567890,
		Type:      "supergroup",
		Title:     "Dupli1 Ops",
	})
	if !strings.Contains(reply, "<code>-1001234567890</code>") {
		t.Fatalf("expected chat id in reply, got %q", reply)
	}
	if !strings.Contains(reply, "Dupli1 Ops") {
		t.Fatalf("expected group title in reply, got %q", reply)
	}
}

func TestUpdateProcessorStartAccepted(t *testing.T) {
	var gotChatID, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/sendMessage") {
			http.NotFound(w, r)
			return
		}
		var body struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotChatID = body.ChatID
		gotText = body.Text
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	access := service.NewTelegramAccess(service.NewTelegramSubscriptions(memory.NewTelegramRepository()), nil)
	_ = access.Refresh(context.Background())

	processor := &telegram.UpdateProcessor{
		Client: client,
		Policy: access,
		Lookup: &stubLookup{sub: &domain.TelegramSubscription{
			ChatID: "42",
			Status: domain.SubscriptionStatusAccepted,
		}},
	}

	update := telegram.Update{
		Message: &telegram.Message{
			Text: "/start",
			From: &telegram.User{ID: 42},
			Chat: telegram.Chat{ID: 42, Type: "private", FirstName: "Alex"},
		},
	}
	if err := processor.Handle(context.Background(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gotChatID != "42" {
		t.Fatalf("chat id = %q, want 42", gotChatID)
	}
	if !strings.Contains(gotText, "Welcome") {
		t.Fatalf("expected welcome reply, got %q", gotText)
	}
}

func TestUpdateProcessorStartPending(t *testing.T) {
	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotText = body.Text
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	processor := &telegram.UpdateProcessor{
		Client: client,
		Lookup: &stubLookup{sub: &domain.TelegramSubscription{
			ChatID: "42",
			Status: domain.SubscriptionStatusPending,
		}},
	}

	update := telegram.Update{
		Message: &telegram.Message{
			Text: "/start",
			Chat: telegram.Chat{ID: 42, Type: "private"},
		},
	}
	if err := processor.Handle(context.Background(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !strings.Contains(gotText, "Registration received") {
		t.Fatalf("expected pending reply, got %q", gotText)
	}
}

func TestUpdateProcessorStartDeniedForUnknownUser(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	access := service.NewTelegramAccess(service.NewTelegramSubscriptions(memory.NewTelegramRepository()), nil)
	if err := access.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	processor := &telegram.UpdateProcessor{
		Client: client,
		Policy: access,
		Lookup: &stubLookup{},
	}
	update := telegram.Update{
		Message: &telegram.Message{
			Text: "/start",
			From: &telegram.User{ID: 999},
			Chat: telegram.Chat{ID: 999, Type: "private", FirstName: "Stranger"},
		},
	}
	if err := processor.Handle(context.Background(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if called {
		t.Fatal("expected no Telegram reply for unknown /start user")
	}
}

func TestUpdateProcessorIgnoresOtherCommands(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	processor := &telegram.UpdateProcessor{Client: client}
	update := telegram.Update{
		Message: &telegram.Message{
			Text: "/help",
			Chat: telegram.Chat{ID: 1, Type: "private"},
		},
	}
	if err := processor.Handle(context.Background(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if called {
		t.Fatal("expected no API call for /help")
	}
}

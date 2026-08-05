package telegram_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
)

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

func TestHandleMessageStartSendsReply(t *testing.T) {
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

	token := "test-token"
	client := telegram.NewTestClient(token, srv.Client(), srv.URL)
	client.SetAllowlist(telegram.NewAllowlist("42", "", "42"))

	msg := &telegram.Message{
		Text: "/start",
		From: &telegram.User{ID: 42},
		Chat: telegram.Chat{ID: 42, Type: "private", FirstName: "Alex"},
	}
	if err := telegram.HandleMessage(context.Background(), client, msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if gotChatID != "42" {
		t.Fatalf("chat id = %q, want 42", gotChatID)
	}
	if !strings.Contains(gotText, "<code>42</code>") {
		t.Fatalf("expected chat id in reply text, got %q", gotText)
	}
}

func TestHandleMessageStartDeniedWhenNotOnList(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	client.SetAllowlist(telegram.NewAllowlist("", "", "42"))

	msg := &telegram.Message{
		Text: "/start",
		From: &telegram.User{ID: 99},
		Chat: telegram.Chat{ID: 99, Type: "private"},
	}
	if err := telegram.HandleMessage(context.Background(), client, msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if called {
		t.Fatal("expected no API call for user not on allowlist")
	}
}

func TestHandleMessageIgnoresOtherCommands(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	msg := &telegram.Message{
		Text: "/help",
		Chat: telegram.Chat{ID: 1, Type: "private"},
	}
	if err := telegram.HandleMessage(context.Background(), client, msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if called {
		t.Fatal("expected no API call for /help")
	}
}

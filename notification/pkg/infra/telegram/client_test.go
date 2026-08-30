package telegram_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
)

func TestClientSendRespectsOutboundAllowlist(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	allowlist := telegram.NewAllowlist("-1001", "", "")
	client.SetAccessPolicy(allowlist)

	if err := client.Send(t.Context(), "42", "blocked"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if called {
		t.Fatal("Send must skip non-allowlisted chat without error")
	}

	called = false
	if err := client.Send(t.Context(), "-1001", "allowed"); err != nil {
		t.Fatalf("Send allowlisted: %v", err)
	}
	if !called {
		t.Fatal("Send must deliver to allowlisted chat")
	}
}

func TestClientReplyBypassesOutboundAllowlist(t *testing.T) {
	var gotChatID, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotChatID = body.ChatID
		gotText = body.Text
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	allowlist := telegram.NewAllowlist("-1001", "", "")
	client.SetAccessPolicy(allowlist)

	if err := client.Reply(t.Context(), "42", "Registration received"); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if gotChatID != "42" {
		t.Fatalf("chat id = %q, want 42", gotChatID)
	}
	if gotText != "Registration received" {
		t.Fatalf("text = %q, want registration ack", gotText)
	}
}

package telegram_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/telegram"
)

// allowChat is the smallest AccessPolicy that exercises the client: one chat
// may receive outbound messages, everything else is refused. The ops bot's
// env-backed Allowlist is one implementation of the same interface and stays
// with notification, which is the only service that needs its shape.
type allowChat string

func (a allowChat) AllowsChat(chatID string) bool { return chatID == string(a) }

func (a allowChat) AllowsIncoming(telegram.Chat, *telegram.User) bool { return true }

func TestClientSendRespectsOutboundAllowlist(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	client.SetAccessPolicy(allowChat("-1001"))

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
	client.SetAccessPolicy(allowChat("-1001"))

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

// botToken is a realistic token shape: BotFather issues <id>:<secret>.
const botToken = "7654321:AAHleakedSecretTokenValue"

// Transport failures are reported by net/http as *url.Error, whose message
// embeds the request URL — and the bot token lives in that URL's path. These
// errors are logged verbatim by the NATS dispatcher and the poller, so the
// token must never survive into their text.
func TestClientErrorsDoNotLeakBotToken(t *testing.T) {
	// Port 1 is closed, so every call fails in the transport like a prod reset.
	client := telegram.NewTestClient(botToken, &http.Client{Timeout: time.Second}, "http://127.0.0.1:1")

	calls := map[string]func() error{
		"Send":          func() error { return client.Send(t.Context(), "-1001", "hello") },
		"Reply":         func() error { return client.Reply(t.Context(), "-1001", "hello") },
		"DeleteWebhook": func() error { return client.DeleteWebhook(t.Context()) },
		"SetWebhook":    func() error { return client.SetWebhook(t.Context(), "https://example.com/hook", "s3cret") },
		"GetUpdates": func() error {
			_, err := client.GetUpdates(t.Context(), 0, 0)
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("expected a transport error")
			}
			if strings.Contains(err.Error(), botToken) {
				t.Fatalf("bot token leaked into error text: %v", err)
			}
		})
	}
}

// Redaction must not break errors.Is/As for callers inspecting the cause.
func TestClientRedactedErrorUnwraps(t *testing.T) {
	client := telegram.NewTestClient(botToken, &http.Client{Timeout: time.Second}, "http://127.0.0.1:1")

	err := client.Send(t.Context(), "-1001", "hello")
	if err == nil {
		t.Fatal("expected a transport error")
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("expected a *url.Error in the chain, got %v", err)
	}
}

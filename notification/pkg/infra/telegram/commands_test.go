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
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
)

type stubLookup struct {
	sub             *domain.TelegramSubscription
	metadataUpdates int
}

func (s *stubLookup) RegisterFromMessage(ctx context.Context, in telegram.SubscriptionInput) (*domain.TelegramSubscription, error) {
	return s.sub, nil
}

func (s *stubLookup) FindForMessage(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error) {
	return s.sub, nil
}

func (s *stubLookup) UpdateMetadata(ctx context.Context, id string, in telegram.SubscriptionInput) error {
	s.metadataUpdates++
	return nil
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
		ID:    -1001234567890,
		Type:  "supergroup",
		Title: "Dupli1 Ops",
	})
	if !strings.Contains(reply, "<code>-1001234567890</code>") {
		t.Fatalf("expected chat id in reply, got %q", reply)
	}
	if !strings.Contains(reply, "그룹: <b>Dupli1 Ops</b>") {
		t.Fatalf("expected Korean group label, got %q", reply)
	}
	if !strings.Contains(reply, "Dupli1 운영 알림") {
		t.Fatalf("expected Korean welcome copy, got %q", reply)
	}
}

func TestFormatPendingReplyIsKorean(t *testing.T) {
	reply := telegram.FormatPendingReply(telegram.Chat{
		ID:        42,
		Type:      "private",
		FirstName: "Alex",
	})
	if !strings.Contains(reply, "등록 요청이 접수되었습니다") {
		t.Fatalf("expected Korean pending copy, got %q", reply)
	}
	if !strings.Contains(reply, "개인 채팅: <b>Alex</b>") {
		t.Fatalf("expected Korean private-chat label, got %q", reply)
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
	_ = access.Refresh(t.Context())

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
	if err := processor.Handle(t.Context(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gotChatID != "42" {
		t.Fatalf("chat id = %q, want 42", gotChatID)
	}
	if !strings.Contains(gotText, "Dupli1 운영 알림") {
		t.Fatalf("expected welcome reply, got %q", gotText)
	}
}

// A chat nobody has seen before registers on its first /start and is
// acknowledged once. The second /start is silent: the row already exists, and
// repeating "a manager must approve this" adds nothing.
func TestUpdateProcessorStartRegistersOnceAndThenStaysQuiet(t *testing.T) {
	var replies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		replies = append(replies, body.Text)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	ctx := t.Context()
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	access := service.NewTelegramAccess(subs, nil)
	if err := access.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	client.SetAccessPolicy(access)
	processor := &telegram.UpdateProcessor{
		Client: client,
		Policy: access,
		// The real lookup, not a stub: a stub that returns nothing from
		// RegisterFromMessage does not model the repository at all.
		Lookup: service.NewSubscriptionLookup(subs),
	}

	update := telegram.Update{
		Message: &telegram.Message{
			Text: "/start",
			From: &telegram.User{ID: 999},
			Chat: telegram.Chat{ID: 999, Type: "private", FirstName: "Stranger"},
		},
	}

	if err := processor.Handle(ctx, update); err != nil {
		t.Fatalf("first /start: %v", err)
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "등록 요청이 접수되었습니다") {
		t.Fatalf("first /start should be acknowledged once, got %q", replies)
	}
	pending, err := subs.List(ctx, domain.SubscriptionStatusPending)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending row, got %d", len(pending))
	}

	if err := processor.Handle(ctx, update); err != nil {
		t.Fatalf("second /start: %v", err)
	}
	if len(replies) != 1 {
		t.Fatalf("second /start should be silent, got %q", replies)
	}
	if pending, err = subs.List(ctx, domain.SubscriptionStatusPending); err != nil || len(pending) != 1 {
		t.Fatalf("second /start should not add a row (err=%v, rows=%d)", err, len(pending))
	}
}

// Anything that is not /start leaves no trace: no row, no reply. A bot sitting
// in a group used to collect a pending row from ordinary chatter.
func TestUpdateProcessorDoesNotRegisterStrayMessages(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	ctx := t.Context()
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	access := service.NewTelegramAccess(subs, nil)
	if err := access.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	processor := &telegram.UpdateProcessor{
		Client: telegram.NewTestClient("test-token", srv.Client(), srv.URL),
		Policy: access,
		Lookup: service.NewSubscriptionLookup(subs),
	}

	update := telegram.Update{
		Message: &telegram.Message{
			Text: "good morning everyone",
			From: &telegram.User{ID: 555},
			Chat: telegram.Chat{ID: -100555, Type: "group", Title: "Some group"},
		},
	}
	if err := processor.Handle(ctx, update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if called {
		t.Fatal("a stray message must not produce a reply")
	}
	rows, err := subs.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a stray message must not register a chat, got %d rows", len(rows))
	}
}

// A user on the env allowlist is already trusted, so /start welcomes them
// without creating a pending row for a manager to approve.
func TestUpdateProcessorEnvAllowlistedUserSkipsRegistration(t *testing.T) {
	var replies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		replies = append(replies, body.Text)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	ctx := t.Context()
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	access := service.NewTelegramAccess(subs, &ports.TelegramEnvAllowlist{AllowedUserIDs: "777"})
	if err := access.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	client.SetAccessPolicy(access)
	processor := &telegram.UpdateProcessor{
		Client: client,
		Policy: access,
		Lookup: service.NewSubscriptionLookup(subs),
	}

	update := telegram.Update{
		Message: &telegram.Message{
			Text: "/start",
			From: &telegram.User{ID: 777},
			Chat: telegram.Chat{ID: 777, Type: "private", FirstName: "Ops"},
		},
	}
	if err := processor.Handle(ctx, update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "Dupli1 운영 알림") {
		t.Fatalf("expected the welcome reply, got %q", replies)
	}
	rows, err := subs.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("an allowlisted user needs no pending row, got %d", len(rows))
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
	if err := processor.Handle(t.Context(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if called {
		t.Fatal("expected no API call for /help")
	}
}

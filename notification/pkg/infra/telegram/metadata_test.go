package telegram_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/service"
)

func startUpdate(chatTitle string, chatID int64) telegram.Update {
	return telegram.Update{
		Message: &telegram.Message{
			Text: "/start",
			From: &telegram.User{ID: 4242, Username: "ops_lead"},
			Chat: telegram.Chat{ID: chatID, Type: "group", Title: chatTitle},
		},
	}
}

// Display fields are captured when a chat registers, so a group renamed later
// kept its old name in the manager UI. /start is the one moment fresh metadata
// arrives.
func TestUpdateProcessorRefreshesRenamedChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	ctx := t.Context()
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	processor := &telegram.UpdateProcessor{
		Client: telegram.NewTestClient("test-token", srv.Client(), srv.URL),
		Lookup: service.NewSubscriptionLookup(subs),
	}

	if err := processor.Handle(ctx, startUpdate("Ops Team", -100777)); err != nil {
		t.Fatalf("first /start: %v", err)
	}
	rows, err := subs.List(ctx, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected one row (err=%v, rows=%d)", err, len(rows))
	}
	if rows[0].ChatLabel != "Ops Team" {
		t.Fatalf("label = %q, want the registered title", rows[0].ChatLabel)
	}

	if err := processor.Handle(ctx, startUpdate("Ops Team (Seoul)", -100777)); err != nil {
		t.Fatalf("second /start: %v", err)
	}
	rows, err = subs.List(ctx, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("refresh should not add a row (err=%v, rows=%d)", err, len(rows))
	}
	if rows[0].ChatLabel != "Ops Team (Seoul)" {
		t.Fatalf("label = %q, want the renamed title", rows[0].ChatLabel)
	}
	if rows[0].Username != "ops_lead" {
		t.Fatalf("username = %q, want it carried over", rows[0].Username)
	}
	if rows[0].Status != domain.SubscriptionStatusPending {
		t.Fatalf("status = %q, a refresh must not change it", rows[0].Status)
	}
}

// An unchanged chat must not write on every /start.
func TestUpdateProcessorSkipsUnchangedMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	lookup := &stubLookup{sub: &domain.TelegramSubscription{
		ID:        "sub-1",
		ChatID:    "-100777",
		ChatType:  "group",
		ChatLabel: "Ops Team",
		Username:  "ops_lead",
		Status:    domain.SubscriptionStatusAccepted,
	}}
	processor := &telegram.UpdateProcessor{
		Client: telegram.NewTestClient("test-token", srv.Client(), srv.URL),
		Lookup: lookup,
	}

	if err := processor.Handle(t.Context(), startUpdate("Ops Team", -100777)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if lookup.metadataUpdates != 0 {
		t.Fatalf("unchanged metadata triggered %d writes", lookup.metadataUpdates)
	}

	if err := processor.Handle(t.Context(), startUpdate("Renamed", -100777)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if lookup.metadataUpdates != 1 {
		t.Fatalf("a rename should write exactly once, got %d", lookup.metadataUpdates)
	}
}

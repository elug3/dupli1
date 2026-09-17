package service_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
)

func orderPayload(t *testing.T, id string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"event_type":  "order.created",
		"order_id":    id,
		"customer_id": "cust-1",
		"status":      "pending",
		"total_won":   1000,
		"occurred_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return payload
}

// Every accepted chat carrying alert_order is a destination. Only the first was
// used before, so a manager could tick "Orders" on three chats and two of them
// would silently never hear anything.
func TestDispatcherAlertsEveryAcceptedChat(t *testing.T) {
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	ctx := t.Context()

	for _, chatID := range []string{"-100one", "-100two", "-100three"} {
		if _, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
			ChatID:     chatID,
			AlertOrder: true,
		}); err != nil {
			t.Fatalf("accept %s: %v", chatID, err)
		}
	}
	// Product-only chats must not receive order alerts.
	if _, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
		ChatID:       "-100products",
		AlertProduct: true,
	}); err != nil {
		t.Fatalf("accept product chat: %v", err)
	}

	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing: service.NewTelegramRouting(subs, &ports.TelegramEnvAllowlist{}),
	})

	if err := dispatcher.HandleForTest(ctx, service.SubjectOrderCreated, orderPayload(t, "ORD-FAN")); err != nil {
		t.Fatalf("handle order: %v", err)
	}

	if len(notifier.chatIDs) != 3 {
		t.Fatalf("sent to %v, want all three order chats", notifier.chatIDs)
	}
	for _, want := range []string{"-100one", "-100two", "-100three"} {
		if !strings.Contains(strings.Join(notifier.chatIDs, ","), want) {
			t.Fatalf("chat %s missed, got %v", want, notifier.chatIDs)
		}
	}
	if strings.Contains(strings.Join(notifier.chatIDs, ","), "-100products") {
		t.Fatalf("product-only chat received an order alert: %v", notifier.chatIDs)
	}
}

// The env chat and accepted chats are unioned. Preferring env meant that while
// TELEGRAM_ORDER_CHAT_ID was set — as it is in production — accepting a chat in
// manage-web changed nothing at all.
func TestDispatcherUnionsEnvAndAcceptedChats(t *testing.T) {
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	ctx := t.Context()

	if _, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
		ChatID:     "-100accepted",
		AlertOrder: true,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	env := &ports.TelegramEnvAllowlist{OrderChatID: "-100env"}
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing:     service.NewTelegramRouting(subs, env),
		OrderChatID: "-100env",
	})

	if err := dispatcher.HandleForTest(ctx, service.SubjectOrderCreated, orderPayload(t, "ORD-UNION")); err != nil {
		t.Fatalf("handle order: %v", err)
	}

	if got := strings.Join(notifier.chatIDs, ","); got != "-100env,-100accepted" {
		t.Fatalf("chats = %q, want the env chat and the accepted chat exactly once each", got)
	}
}

// A chat reachable through both env and the database is alerted once.
func TestDispatcherDoesNotDuplicateSharedChat(t *testing.T) {
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	ctx := t.Context()

	if _, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
		ChatID:     "-100same",
		AlertOrder: true,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	env := &ports.TelegramEnvAllowlist{OrderChatID: "-100same"}
	notifier := &recordedNotifier{}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing:     service.NewTelegramRouting(subs, env),
		OrderChatID: "-100same",
	})

	if err := dispatcher.HandleForTest(ctx, service.SubjectOrderCreated, orderPayload(t, "ORD-DUP")); err != nil {
		t.Fatalf("handle order: %v", err)
	}
	if len(notifier.chatIDs) != 1 {
		t.Fatalf("sent %d times, want one: %v", len(notifier.chatIDs), notifier.chatIDs)
	}
}

// One unreachable chat must not silence the others: every destination is
// attempted, and the failure is still reported.
func TestDispatcherKeepsSendingAfterAFailedChat(t *testing.T) {
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	ctx := t.Context()

	for _, chatID := range []string{"-100bad", "-100good"} {
		if _, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
			ChatID:     chatID,
			AlertOrder: true,
		}); err != nil {
			t.Fatalf("accept %s: %v", chatID, err)
		}
	}

	sendErr := errors.New("chat not found")
	notifier := &recordedNotifier{err: sendErr, failFor: "-100bad"}
	dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
		Routing: service.NewTelegramRouting(subs, &ports.TelegramEnvAllowlist{}),
	})

	err := dispatcher.HandleForTest(ctx, service.SubjectOrderCreated, orderPayload(t, "ORD-PARTIAL"))
	if err == nil {
		t.Fatal("expected the failed chat to be reported")
	}
	if !errors.Is(err, sendErr) {
		t.Fatalf("error should wrap the send failure, got %v", err)
	}
	if len(notifier.chatIDs) != 2 {
		t.Fatalf("every chat should have been attempted, got %v", notifier.chatIDs)
	}
}

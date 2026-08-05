package service_test

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
)

func TestTelegramSubscriptionsAcceptAndRoute(t *testing.T) {
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	ctx := context.Background()

	pending, err := subs.RegisterFromMessage(ctx, ports.TelegramSubscriptionInput{
		TelegramUserID: int64Ptr(42),
		ChatID:         "42",
		ChatType:       "private",
		ChatLabel:      "Alex",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if pending.Status != domain.SubscriptionStatusPending {
		t.Fatalf("status = %q, want pending", pending.Status)
	}

	accepted, err := subs.Accept(ctx, pending.ID, ports.TelegramAcceptInput{
		AlertOrder:   true,
		AlertProduct: false,
		AcceptedBy:   "manager-1",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if !accepted.AlertOrder {
		t.Fatal("expected alert_order true")
	}

	env := &ports.TelegramEnvAllowlist{}
	order, product := subs.RoutingChats(ctx, env)
	if order != "42" {
		t.Fatalf("order chat = %q, want 42", order)
	}
	if product != "" {
		t.Fatalf("product chat = %q, want empty", product)
	}

	access := service.NewTelegramAccess(subs, env)
	if err := access.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !access.AllowsIncoming(telegram.Chat{ID: 42, Type: "private"}, &telegram.User{ID: 42}) {
		t.Fatal("expected accepted user to be allowed")
	}
}

func TestTelegramSubscriptionsManualChatID(t *testing.T) {
	subs := service.NewTelegramSubscriptions(memory.NewTelegramRepository())
	item, err := subs.CreateManual(context.Background(), ports.TelegramManualInput{
		ChatID:       "-100999",
		ChatLabel:    "Ops",
		AlertProduct: true,
		AcceptedBy:   "manager-1",
	})
	if err != nil {
		t.Fatalf("create manual: %v", err)
	}
	if item.Status != domain.SubscriptionStatusAccepted {
		t.Fatalf("status = %q", item.Status)
	}
}

func int64Ptr(v int64) *int64 { return &v }

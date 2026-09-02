package service_test

import (
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
	ctx := t.Context()

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
	item, err := subs.CreateManual(t.Context(), ports.TelegramManualInput{
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

func TestTelegramSubscriptionsRejectAndLookup(t *testing.T) {
	repo := memory.NewTelegramRepository()
	subs := service.NewTelegramSubscriptions(repo)
	ctx := t.Context()

	pending, err := subs.RegisterFromMessage(ctx, ports.TelegramSubscriptionInput{
		TelegramUserID: int64Ptr(55),
		ChatID:         "55",
		ChatType:       "private",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	rejected, err := subs.Reject(ctx, pending.ID, "manager-1")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != domain.SubscriptionStatusRejected {
		t.Fatalf("status = %q, want rejected", rejected.Status)
	}

	sub, err := subs.LookupForMessage(ctx, "55", int64Ptr(55))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if sub == nil || sub.Status != domain.SubscriptionStatusRejected {
		t.Fatalf("lookup after reject: %+v", sub)
	}

	env := &ports.TelegramEnvAllowlist{}
	if subs.IsAllowedIncoming(ctx, "55", int64Ptr(55), env) {
		t.Fatal("rejected subscription should not allow incoming")
	}
}

func TestTelegramSubscriptionsIsAllowedIncomingEnvAllowlist(t *testing.T) {
	subs := service.NewTelegramSubscriptions(memory.NewTelegramRepository())
	ctx := t.Context()
	env := &ports.TelegramEnvAllowlist{
		AllowedUserIDs: "123",
		OrderChatID:    "-100777",
	}

	if !subs.IsAllowedIncoming(ctx, "999", int64Ptr(123), env) {
		t.Fatal("expected env user allowlist to permit incoming")
	}
	if !subs.IsAllowedIncoming(ctx, "-100777", nil, env) {
		t.Fatal("expected env order chat allowlist to permit incoming")
	}
	if subs.IsAllowedIncoming(ctx, "999", int64Ptr(456), env) {
		t.Fatal("expected unknown user to be denied")
	}
}

func TestTelegramAccessDeniesUnknownAfterRefresh(t *testing.T) {
	subs := service.NewTelegramSubscriptions(memory.NewTelegramRepository())
	access := service.NewTelegramAccess(subs, &ports.TelegramEnvAllowlist{})
	if err := access.Refresh(t.Context()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if access.AllowsIncoming(telegram.Chat{ID: 404, Type: "private"}, &telegram.User{ID: 404}) {
		t.Fatal("expected unknown user to be denied")
	}
}

func int64Ptr(v int64) *int64 { return &v }

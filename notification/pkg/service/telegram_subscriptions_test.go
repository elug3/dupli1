package service_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/telegram"
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
	order, product := subs.RoutingChats(ctx, env, "")
	if len(order) != 1 || order[0] != "42" {
		t.Fatalf("order chats = %v, want [42]", order)
	}
	if len(product) != 0 {
		t.Fatalf("product chats = %v, want none", product)
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

// Changing an accepted chat's alerts reroutes it on the next dispatch.
func TestTelegramSubscriptionsUpdateAlertsReroutes(t *testing.T) {
	subs := service.NewTelegramSubscriptions(memory.NewTelegramRepository())
	ctx := t.Context()
	item, err := subs.CreateManual(ctx, ports.TelegramManualInput{
		ChatID:     "-100555",
		AlertOrder: true,
		AcceptedBy: "manager-1",
	})
	if err != nil {
		t.Fatalf("create manual: %v", err)
	}

	off, on := false, true
	if _, err := subs.UpdateAlerts(ctx, item.ID, ports.TelegramAlertsInput{AlertOrder: &off, AlertProduct: &on}); err != nil {
		t.Fatalf("update alerts: %v", err)
	}
	order, product := subs.RoutingChats(ctx, &ports.TelegramEnvAllowlist{}, "")
	if len(order) != 0 {
		t.Fatalf("order chats = %v, want none", order)
	}
	if len(product) != 1 || product[0] != "-100555" {
		t.Fatalf("product chats = %v, want [-100555]", product)
	}

	if _, err := subs.UpdateAlerts(ctx, item.ID, ports.TelegramAlertsInput{}); !errors.Is(err, service.ErrNoAlertChange) {
		t.Fatalf("empty update err = %v, want ErrNoAlertChange", err)
	}
}

// Muted events are validated, sorted and de-duplicated; an empty list unmutes
// everything, and leaving the field out keeps what is there.
func TestTelegramSubscriptionsMutedEvents(t *testing.T) {
	subs := service.NewTelegramSubscriptions(memory.NewTelegramRepository())
	ctx := t.Context()
	item, err := subs.CreateManual(ctx, ports.TelegramManualInput{ChatID: "-100mute", AlertOrder: true})
	if err != nil {
		t.Fatalf("create manual: %v", err)
	}
	if item.MutedEvents == nil || len(item.MutedEvents) != 0 {
		t.Fatalf("new subscription muted = %#v, want an empty list", item.MutedEvents)
	}

	muted := []string{"order.status_updated", "order.created", "order.created"}
	got, err := subs.UpdateAlerts(ctx, item.ID, ports.TelegramAlertsInput{MutedEvents: &muted})
	if err != nil {
		t.Fatalf("mute: %v", err)
	}
	if want := "order.created,order.status_updated"; strings.Join(got.MutedEvents, ",") != want {
		t.Fatalf("muted = %v, want %s", got.MutedEvents, want)
	}

	on := true
	got, err = subs.UpdateAlerts(ctx, item.ID, ports.TelegramAlertsInput{AlertProduct: &on})
	if err != nil {
		t.Fatalf("flag update: %v", err)
	}
	if len(got.MutedEvents) != 2 {
		t.Fatalf("muted after a flag-only update = %v, want it kept", got.MutedEvents)
	}

	order, _ := subs.RoutingChats(ctx, &ports.TelegramEnvAllowlist{}, "order.created")
	if len(order) != 0 {
		t.Fatalf("order.created chats = %v, want none", order)
	}
	order, _ = subs.RoutingChats(ctx, &ports.TelegramEnvAllowlist{}, "order.paid")
	if len(order) != 1 {
		t.Fatalf("order.paid chats = %v, want [-100mute]", order)
	}

	for _, bad := range [][]string{{"order.shipped"}, {"support.inquiry_opened"}} {
		if _, err := subs.UpdateAlerts(ctx, item.ID, ports.TelegramAlertsInput{MutedEvents: &bad}); !errors.Is(err, service.ErrUnknownEvent) {
			t.Fatalf("mute %v err = %v, want ErrUnknownEvent", bad, err)
		}
	}

	none := []string{}
	got, err = subs.UpdateAlerts(ctx, item.ID, ports.TelegramAlertsInput{MutedEvents: &none})
	if err != nil {
		t.Fatalf("unmute: %v", err)
	}
	if len(got.MutedEvents) != 0 {
		t.Fatalf("muted after clearing = %v, want none", got.MutedEvents)
	}
}

// The domain spells out the mutable subjects because it imports nothing; they
// must be exactly what the dispatcher alerts on, or a mute silences nothing.
func TestMutableEventsMatchDispatchedSubjects(t *testing.T) {
	order := []string{
		service.SubjectOrderCreated, service.SubjectOrderPaid, service.SubjectOrderStatusUpdate,
		service.SubjectPaymentCanceled, service.SubjectPaymentCallbackRejected,
	}
	product := []string{
		service.SubjectProductCreated, service.SubjectProductUpdated,
		service.SubjectProductDeleted, service.SubjectProductImage,
	}
	sameSet := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		seen := map[string]bool{}
		for _, v := range a {
			seen[v] = true
		}
		for _, v := range b {
			if !seen[v] {
				return false
			}
		}
		return true
	}
	if !sameSet(domain.OrderAlertEvents, order) {
		t.Fatalf("OrderAlertEvents = %v, want %v", domain.OrderAlertEvents, order)
	}
	if !sameSet(domain.ProductAlertEvents, product) {
		t.Fatalf("ProductAlertEvents = %v, want %v", domain.ProductAlertEvents, product)
	}
}

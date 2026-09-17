package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
)

// A manager accepting a chat updates the database, but only the task that
// served the request runs its OnSubscriptionsChanged callback. Every other task
// has to notice the change on its own, or it keeps denying an accepted chat
// until it restarts.
func TestRunRefresherPicksUpOutOfProcessAccepts(t *testing.T) {
	repo := memory.NewTelegramRepository()
	access := service.NewTelegramAccess(service.NewTelegramSubscriptions(repo), &ports.TelegramEnvAllowlist{})

	ctx := t.Context()
	if err := access.Refresh(ctx); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if access.AllowsChat("-100777") {
		t.Fatal("chat should not be allowed before it is accepted")
	}

	go access.RunRefresher(ctx, time.Millisecond)

	// Stands in for another task writing to the shared database.
	if _, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
		ChatID:     "-100777",
		AlertOrder: true,
		AcceptedBy: "another-task",
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !access.AllowsChat("-100777") {
		if time.Now().After(deadline) {
			t.Fatal("refresher did not pick up the accepted chat")
		}
		time.Sleep(time.Millisecond)
	}
}

// A zero interval must not spin: it selects the package default.
func TestRunRefresherRejectsZeroInterval(t *testing.T) {
	repo := memory.NewTelegramRepository()
	access := service.NewTelegramAccess(service.NewTelegramSubscriptions(repo), &ports.TelegramEnvAllowlist{})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		access.RunRefresher(ctx, 0)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunRefresher did not stop when its context was cancelled")
	}
}

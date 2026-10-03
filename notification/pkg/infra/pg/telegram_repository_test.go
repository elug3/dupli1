package pg

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/ports"
)

func openNotificationRepo(t *testing.T) *TelegramRepository {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://dupli1:dupli1_dev@localhost:5438/notifications?sslmode=disable"
	}
	repo, err := NewTelegramRepository(dsn)
	if err != nil {
		t.Skipf("notification postgres unavailable: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

// CreateAccepted used to put chat_label into username, which made manage-web
// show "@Ops" with no label until the chat sent /start.
func TestCreateAccepted_StoresChatLabelNotUsername(t *testing.T) {
	repo := openNotificationRepo(t)
	ctx := t.Context()
	chatID := fmt.Sprintf("-100test-%s", t.Name())
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(context.Background(), `DELETE FROM telegram_subscriptions WHERE chat_id = $1`, chatID)
	})

	got, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
		ChatID:     chatID,
		ChatLabel:  "Ops console",
		AlertOrder: true,
		AcceptedBy: "manager-1",
	})
	if err != nil {
		t.Fatalf("CreateAccepted: %v", err)
	}
	if got.ChatLabel != "Ops console" || got.Username != "" {
		t.Fatalf("CreateAccepted returned label=%q username=%q", got.ChatLabel, got.Username)
	}

	stored, err := repo.FindByChatID(ctx, chatID)
	if err != nil {
		t.Fatalf("FindByChatID: %v", err)
	}
	if stored.ChatLabel != "Ops console" || stored.Username != "" {
		t.Fatalf("stored row label=%q username=%q, want label Ops console and empty username", stored.ChatLabel, stored.Username)
	}
}

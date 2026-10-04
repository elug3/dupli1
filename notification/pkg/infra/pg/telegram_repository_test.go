package pg_test

import (
	"os"
	"strings"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/infra/pg"
	"github.com/elug3/dupli1/notification/pkg/ports"
)

func TestCreateAccepted_StoresManualLabelNotUsername(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set")
	}

	repo, err := pg.NewTelegramRepository(dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(repo.Close)

	ctx := t.Context()
	chatID := "-100" + strings.ReplaceAll(strings.ReplaceAll(t.Name(), "/", ""), " ", "")
	sub, err := repo.CreateAccepted(ctx, ports.TelegramManualInput{
		ChatID:     chatID,
		ChatLabel:  "Finance desk",
		AlertOrder: true,
		AcceptedBy: "manager-1",
	})
	if err != nil {
		t.Fatalf("CreateAccepted: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(ctx, sub.ID) })

	got, err := repo.GetByID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ChatLabel != "Finance desk" {
		t.Fatalf("chat_label = %q, want the manager-entered label", got.ChatLabel)
	}
	if got.Username != "" {
		t.Fatalf("username = %q, want empty (label must not land in username)", got.Username)
	}
}

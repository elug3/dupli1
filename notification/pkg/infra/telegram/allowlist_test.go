package telegram_test

import (
	"testing"

	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
)

func TestAllowlistChatIDs(t *testing.T) {
	list := telegram.NewAllowlist("-1001", "-1002", "")
	if !list.AllowsChat("-1001") {
		t.Fatal("expected order chat to be allowed")
	}
	if list.AllowsChat("-9999") {
		t.Fatal("expected unknown chat to be denied")
	}
}

func TestAllowlistUserIDs(t *testing.T) {
	list := telegram.NewAllowlist("", "", "123, 456")
	if !list.AllowsUser(123) {
		t.Fatal("expected user 123 to be allowed")
	}
	if list.AllowsUser(789) {
		t.Fatal("expected user 789 to be denied")
	}
}

func TestAllowsIncomingUserOrChat(t *testing.T) {
	list := telegram.NewAllowlist("-1001", "", "42")

	if !list.AllowsIncoming(telegram.Chat{ID: 42, Type: "private"}, &telegram.User{ID: 42}) {
		t.Fatal("expected allowed user in private chat")
	}
	if list.AllowsIncoming(telegram.Chat{ID: 99, Type: "private"}, &telegram.User{ID: 99}) {
		t.Fatal("expected unknown user to be denied")
	}
	if !list.AllowsIncoming(telegram.Chat{ID: -1001, Type: "supergroup"}, nil) {
		t.Fatal("expected allowed group chat")
	}
}

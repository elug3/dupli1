package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/pgsslmode"
	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/infra/postgres"

	_ "github.com/lib/pq"
)

func requirePostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set; skipping postgres integration test")
	}
	db, err := sql.Open("postgres", pgsslmode.WithSSLMode(dsn))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := postgres.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// freshChat returns ids unique to the calling test, and clears any row a
// previous run left behind. Both keys are namespaced: chat_id is unique, and so
// is the conversation id, so two tests cannot collide on either.
func freshChat(t *testing.T, db *sql.DB) (chatID, convID string) {
	t.Helper()
	chatID, convID = t.Name()+"-chat", t.Name()+"-conv"
	if _, err := db.Exec(
		`DELETE FROM support_conversations WHERE chat_id = $1 OR id LIKE $2`,
		chatID, t.Name()+"-%",
	); err != nil {
		t.Fatalf("clean conversation: %v", err)
	}
	return chatID, convID
}

func TestMigrateIsIdempotent(t *testing.T) {
	// Every service here migrates inline on every start, so the second run is
	// the normal case, not an edge one.
	db := requirePostgres(t)
	if err := postgres.Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestConversationRoundTrips(t *testing.T) {
	db := requirePostgres(t)
	repo := postgres.NewConversationRepository(db)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)

	if found, err := repo.FindByChatID(ctx, chatID); err != nil || found != nil {
		t.Fatalf("unknown chat = (%v, %v), want (nil, nil)", found, err)
	}

	userID := int64(99)
	now := time.Now().UTC().Truncate(time.Millisecond)
	conversation := domain.NewConversation(convID, chatID, now)
	conversation.TelegramUserID = &userID
	conversation.Username = "shopper"
	conversation.EntryPayload = "c-louis-vuitton-ko"
	conversation.Node = domain.NodeReturn

	if err := repo.Save(ctx, conversation); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := repo.FindByChatID(ctx, chatID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.ID != convID || got.Node != domain.NodeReturn || got.EntryPayload != "c-louis-vuitton-ko" {
		t.Fatalf("conversation = %+v", got)
	}
	if got.TelegramUserID == nil || *got.TelegramUserID != userID || got.Username != "shopper" {
		t.Fatalf("identity = %+v", got)
	}
	if !got.LastSeenAt.UTC().Equal(now) {
		t.Fatalf("last seen = %s, want %s", got.LastSeenAt.UTC(), now)
	}
}

func TestSaveUpsertsOnChatIDNotID(t *testing.T) {
	// Telegram can deliver a message and a button tap close enough together
	// that both try to create the chat's first row. Conflicting on chat_id is
	// what keeps that from becoming two conversations for one shopper.
	db := requirePostgres(t)
	repo := postgres.NewConversationRepository(db)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)
	now := time.Now().UTC()

	first := domain.NewConversation(convID+"-first", chatID, now)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("save first: %v", err)
	}
	second := domain.NewConversation(convID+"-second", chatID, now)
	second.Node = domain.NodeAgent
	if err := repo.Save(ctx, second); err != nil {
		t.Fatalf("save second: %v", err)
	}

	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM support_conversations WHERE chat_id = $1`, chatID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want one conversation per chat", rows)
	}
	got, _ := repo.FindByChatID(ctx, chatID)
	if got.ID != convID+"-first" {
		t.Fatalf("id = %q, want the original row kept", got.ID)
	}
	if got.Node != domain.NodeAgent {
		t.Fatalf("node = %q, want the newer position", got.Node)
	}
}

func TestSaveKeepsIdentityItAlreadyKnows(t *testing.T) {
	// A button tap carries the user, a channel post may not. An update without
	// identity must not blank out what an earlier one established.
	db := requirePostgres(t)
	repo := postgres.NewConversationRepository(db)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)
	now := time.Now().UTC()

	userID := int64(77)
	known := domain.NewConversation(convID, chatID, now)
	known.TelegramUserID = &userID
	known.Username = "shopper"
	if err := repo.Save(ctx, known); err != nil {
		t.Fatalf("save: %v", err)
	}

	anonymous := domain.NewConversation(convID, chatID, now)
	if err := repo.Save(ctx, anonymous); err != nil {
		t.Fatalf("save anonymous: %v", err)
	}

	got, _ := repo.FindByChatID(ctx, chatID)
	if got.TelegramUserID == nil || *got.TelegramUserID != userID || got.Username != "shopper" {
		t.Fatalf("identity lost: %+v", got)
	}
}

func TestSeedFillsMissingAnswersAndNeverOverwritesEdits(t *testing.T) {
	db := requirePostgres(t)
	ctx := context.Background()
	if _, err := db.Exec(`DELETE FROM support_answers`); err != nil {
		t.Fatalf("clean answers: %v", err)
	}

	seeded, err := postgres.SeedAnswers(ctx, db, domain.DefaultLanguage)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seeded != len(domain.DefaultAnswers) {
		t.Fatalf("seeded %d rows, want %d", seeded, len(domain.DefaultAnswers))
	}

	// Staff edit the copy from the inbox…
	edited := "<b>운영팀이 고친 안내</b>"
	if _, err := db.Exec(
		`UPDATE support_answers SET body = $1 WHERE node = $2 AND language = $3`,
		edited, domain.NodeReturn, domain.DefaultLanguage,
	); err != nil {
		t.Fatalf("edit: %v", err)
	}

	// …and the next deploy must not undo it.
	again, err := postgres.SeedAnswers(ctx, db, domain.DefaultLanguage)
	if err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if again != 0 {
		t.Fatalf("reseed wrote %d rows, want none", again)
	}

	body, err := postgres.NewAnswerRepository(db).Body(ctx, domain.NodeReturn, domain.DefaultLanguage)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if body != edited {
		t.Fatalf("body = %q, want the edit preserved", body)
	}
}

func TestAnswerBodyIsEmptyForAnUnknownNode(t *testing.T) {
	db := requirePostgres(t)
	body, err := postgres.NewAnswerRepository(db).Body(context.Background(), "no.such.node", domain.DefaultLanguage)
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if body != "" {
		t.Fatalf("body = %q, want empty so the seeded fallback takes over", body)
	}
}

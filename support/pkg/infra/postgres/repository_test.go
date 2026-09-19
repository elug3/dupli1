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
//
// Messages and inquiries point at the conversation with a plain foreign key, so
// they are cleared first: deleting the parent alone succeeds only against a
// database no earlier run has touched, which is not the database anyone
// actually runs these against twice.
func freshChat(t *testing.T, db *sql.DB) (chatID, convID string) {
	t.Helper()
	chatID, convID = t.Name()+"-chat", t.Name()+"-conv"
	owned := `SELECT id FROM support_conversations WHERE chat_id = $1 OR id LIKE $2`
	for _, child := range []string{"support_messages", "support_inquiries"} {
		if _, err := db.Exec(
			`DELETE FROM `+child+` WHERE conversation_id IN (`+owned+`)`,
			chatID, t.Name()+"-%",
		); err != nil {
			t.Fatalf("clean %s: %v", child, err)
		}
	}
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

func TestAMessageCanBeRecordedOnAChatsVeryFirstUpdate(t *testing.T) {
	// support_messages carries a foreign key to support_conversations, so the
	// conversation has to be saved before anything references it. Getting the
	// order wrong took down the whole update — menu included — the first time a
	// shopper ever wrote, and only a real database showed it.
	db := requirePostgres(t)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)
	now := time.Now().UTC()

	conversations := postgres.NewConversationRepository(db)
	messages := postgres.NewMessageRepository(db)

	if err := conversations.Save(ctx, domain.NewConversation(convID, chatID, now)); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	err := messages.Append(ctx, &domain.Message{
		ID:             convID + "-msg",
		ConversationID: convID,
		Direction:      domain.DirectionInbound,
		Body:           "주문번호 01HXYZ 인데 배송이 안 와요",
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	body, err := messages.LastInbound(ctx, convID)
	if err != nil {
		t.Fatalf("last inbound: %v", err)
	}
	if body != "주문번호 01HXYZ 인데 배송이 안 와요" {
		t.Fatalf("body = %q", body)
	}
}

func TestOnlyOneInquiryPerChatCanBeOpen(t *testing.T) {
	// The router checks before inserting, but two taps landing together would
	// both pass that check. The partial unique index is what actually stops a
	// shopper being queued twice.
	db := requirePostgres(t)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)
	now := time.Now().UTC()

	if err := postgres.NewConversationRepository(db).Save(ctx, domain.NewConversation(convID, chatID, now)); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	inquiries := postgres.NewInquiryRepository(db)
	if err := inquiries.Save(ctx, domain.NewInquiry(convID+"-a", convID, chatID, domain.NodeAgent, now)); err != nil {
		t.Fatalf("first inquiry: %v", err)
	}
	if err := inquiries.Save(ctx, domain.NewInquiry(convID+"-b", convID, chatID, domain.NodeAgent, now)); err == nil {
		t.Fatal("a second open inquiry for one chat must be refused by the database")
	}

	// Closing the first frees the chat for a later, separate consultation.
	first, err := inquiries.FindOpenByChatID(ctx, chatID)
	if err != nil || first == nil {
		t.Fatalf("find open: (%v, %v)", first, err)
	}
	closed := now.Add(time.Hour)
	first.Status, first.ClosedAt = domain.InquiryClosed, &closed
	if err := inquiries.Save(ctx, first); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := inquiries.Save(ctx, domain.NewInquiry(convID+"-b", convID, chatID, domain.NodeAgent, closed)); err != nil {
		t.Fatalf("second consultation after closing: %v", err)
	}
}

func TestPurgeDropsWordsAndKeepsTheRecord(t *testing.T) {
	// Retention is a promise to shoppers. The words go; the shape of the
	// consultation — how many messages, when, from whom, and the inquiry
	// itself — stays, because that is the business record.
	db := requirePostgres(t)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)
	now := time.Now().UTC()

	if err := postgres.NewConversationRepository(db).Save(ctx, domain.NewConversation(convID, chatID, now)); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	inquiries := postgres.NewInquiryRepository(db)
	inquiry := domain.NewInquiry(convID+"-inq", convID, chatID, domain.NodeAgent, now.Add(-200*24*time.Hour))
	if err := inquiries.Save(ctx, inquiry); err != nil {
		t.Fatalf("save inquiry: %v", err)
	}

	messages := postgres.NewMessageRepository(db)
	old := &domain.Message{
		ID: convID + "-old", ConversationID: convID, InquiryID: inquiry.ID,
		Direction: domain.DirectionInbound,
		Body:      "제 연락처는 010-1234-5678 입니다",
		CreatedAt: now.Add(-200 * 24 * time.Hour),
	}
	recent := &domain.Message{
		ID: convID + "-recent", ConversationID: convID, InquiryID: inquiry.ID,
		Direction: domain.DirectionOutbound, Author: "manager-1",
		Body: "확인해 드리겠습니다", Delivery: domain.DeliverySent,
		CreatedAt: now.Add(-10 * 24 * time.Hour),
	}
	for _, message := range []*domain.Message{old, recent} {
		if err := messages.Append(ctx, message); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	purged, err := messages.PurgeBodies(ctx, now.Add(-180*24*time.Hour), domain.PurgedBody)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged %d, want only the message past its window", purged)
	}

	transcript, err := messages.Transcript(ctx, convID)
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(transcript) != 2 {
		t.Fatalf("transcript has %d rows, want both kept", len(transcript))
	}
	if transcript[0].Body != domain.PurgedBody {
		t.Fatalf("expired body = %q, want the placeholder", transcript[0].Body)
	}
	if transcript[0].Direction != domain.DirectionInbound || transcript[0].CreatedAt.IsZero() {
		t.Fatalf("purge destroyed the record: %+v", transcript[0])
	}
	if transcript[1].Body != "확인해 드리겠습니다" || transcript[1].Author != "manager-1" {
		t.Fatalf("a message inside the window was touched: %+v", transcript[1])
	}
	if open, err := inquiries.FindByID(ctx, inquiry.ID); err != nil || open == nil {
		t.Fatalf("inquiry metadata must survive the purge: (%v, %v)", open, err)
	}
}

func TestPurgeIsIdempotent(t *testing.T) {
	// It sweeps daily and once at start, so re-running must not churn rows it
	// has already handled.
	db := requirePostgres(t)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)
	now := time.Now().UTC()

	if err := postgres.NewConversationRepository(db).Save(ctx, domain.NewConversation(convID, chatID, now)); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	messages := postgres.NewMessageRepository(db)
	err := messages.Append(ctx, &domain.Message{
		ID: convID + "-old", ConversationID: convID, Direction: domain.DirectionInbound,
		Body: "오래된 메시지", CreatedAt: now.Add(-200 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	cutoff := now.Add(-180 * 24 * time.Hour)
	if purged, _ := messages.PurgeBodies(ctx, cutoff, domain.PurgedBody); purged != 1 {
		t.Fatalf("first sweep purged %d, want 1", purged)
	}
	if purged, _ := messages.PurgeBodies(ctx, cutoff, domain.PurgedBody); purged != 0 {
		t.Fatalf("second sweep purged %d, want none", purged)
	}
}

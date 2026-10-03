package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/infra/postgres"
	"github.com/elug3/dupli1/support/pkg/ports"
)

func TestWebConversationRoundTripsItsCustomer(t *testing.T) {
	db := requirePostgres(t)
	ctx := context.Background()
	_, convID := freshChat(t, db)
	chatID := domain.WebChatID(t.Name())
	if _, err := db.Exec(`DELETE FROM support_conversations WHERE chat_id = $1`, chatID); err != nil {
		t.Fatalf("clean: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	conversations := postgres.NewConversationRepository(db)
	conv := domain.NewConversation(convID, chatID, now)
	conv.Channel = domain.ChannelWeb
	conv.CustomerID = t.Name()
	conv.CustomerEmail = "shopper@example.com"
	if err := conversations.Save(ctx, conv); err != nil {
		t.Fatalf("save: %v", err)
	}

	read := now.Add(time.Minute)
	conv.CustomerLastReadAt = &read
	if err := conversations.Save(ctx, conv); err != nil {
		t.Fatalf("save read mark: %v", err)
	}

	got, err := conversations.FindByChatID(ctx, chatID)
	if err != nil || got == nil {
		t.Fatalf("find = (%v, %v)", got, err)
	}
	if got.Channel != domain.ChannelWeb || got.CustomerID != t.Name() || got.CustomerEmail != "shopper@example.com" {
		t.Fatalf("conversation = %+v", got)
	}
	if got.CustomerLastReadAt == nil || !got.CustomerLastReadAt.Equal(read) {
		t.Fatalf("read mark = %v, want %v", got.CustomerLastReadAt, read)
	}

	if err := conversations.ForgetCustomer(ctx, convID); err != nil {
		t.Fatalf("forget: %v", err)
	}
	got, _ = conversations.FindByID(ctx, convID)
	if got.CustomerEmail != "" {
		t.Fatalf("email kept after ForgetCustomer: %q", got.CustomerEmail)
	}
}

func TestWebInquiryAndCardsRoundTrip(t *testing.T) {
	db := requirePostgres(t)
	ctx := context.Background()
	chatID, convID := freshChat(t, db)
	now := time.Now().UTC().Truncate(time.Microsecond)

	conversations := postgres.NewConversationRepository(db)
	inquiries := postgres.NewInquiryRepository(db)
	messages := postgres.NewMessageRepository(db)

	conv := domain.NewConversation(convID, chatID, now)
	conv.Channel = domain.ChannelWeb
	conv.CustomerID = "cust-" + t.Name()
	conv.CustomerEmail = "shopper@example.com"
	if err := conversations.Save(ctx, conv); err != nil {
		t.Fatalf("save conversation: %v", err)
	}

	inq := domain.NewInquiry(convID+"-inq", convID, chatID, domain.NodeProduct, now)
	inq.Channel = domain.ChannelWeb
	inq.CustomerID = conv.CustomerID
	inq.ProductID, inq.SkuID, inq.OrderID = "P01", "SKU01", "ORD1"
	if err := inquiries.Save(ctx, inq); err != nil {
		t.Fatalf("save inquiry: %v", err)
	}
	got, err := inquiries.FindByID(ctx, inq.ID)
	if err != nil || got == nil {
		t.Fatalf("find inquiry = (%v, %v)", got, err)
	}
	if got.Channel != domain.ChannelWeb || got.CustomerID != conv.CustomerID || got.SkuID != "SKU01" || got.OrderID != "ORD1" || got.ProductID != "P01" {
		t.Fatalf("inquiry = %+v", got)
	}

	web, err := inquiries.List(ctx, ports.InquiryFilter{Channel: domain.ChannelWeb})
	if err != nil {
		t.Fatalf("list web: %v", err)
	}
	found := false
	for _, row := range web {
		found = found || row.ID == inq.ID
	}
	if !found {
		t.Fatalf("web inquiry missing from the web list")
	}
	telegram, _ := inquiries.List(ctx, ports.InquiryFilter{Channel: domain.ChannelTelegram})
	for _, row := range telegram {
		if row.ID == inq.ID {
			t.Fatalf("web inquiry in the telegram list")
		}
	}

	card := domain.Message{
		ID: convID + "-card", ConversationID: convID, InquiryID: inq.ID,
		Direction: domain.DirectionInbound, Kind: domain.MessageProductRef,
		Body: "상품: Prada Galleria", RefID: "SKU01",
		RefSnapshot: []byte(`{"sku_id":"SKU01","name":"Prada Galleria"}`), CreatedAt: now,
	}
	reply := domain.Message{
		ID: convID + "-reply", ConversationID: convID, InquiryID: inq.ID,
		Direction: domain.DirectionOutbound, Author: "manager-1", Kind: domain.MessageText,
		Body: "재입고 예정입니다", Delivery: domain.DeliverySent, NoticeStatus: domain.NoticePending,
		CreatedAt: now.Add(time.Second),
	}
	for _, m := range []*domain.Message{&card, &reply} {
		if err := messages.Append(ctx, m); err != nil {
			t.Fatalf("append %s: %v", m.ID, err)
		}
	}

	transcript, err := messages.Transcript(ctx, convID)
	if err != nil || len(transcript) != 2 {
		t.Fatalf("transcript = (%+v, %v)", transcript, err)
	}
	if transcript[0].Kind != domain.MessageProductRef || transcript[0].RefID != "SKU01" || len(transcript[0].RefSnapshot) == 0 {
		t.Fatalf("card = %+v", transcript[0])
	}
	if transcript[1].NoticeStatus != domain.NoticePending {
		t.Fatalf("reply notice = %q", transcript[1].NoticeStatus)
	}

	due, err := messages.DueNotices(ctx, now.Add(time.Hour), 1000)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if !containsMessage(due, reply.ID) || containsMessage(due, card.ID) {
		t.Fatalf("due = %+v, want the pending reply only", due)
	}
	if err := messages.SetNoticeStatus(ctx, []string{reply.ID}, domain.NoticeSent); err != nil {
		t.Fatalf("set notice: %v", err)
	}
	due, _ = messages.DueNotices(ctx, now.Add(time.Hour), 1000)
	if containsMessage(due, reply.ID) {
		t.Fatalf("a sent notice is still due")
	}

	purged, err := messages.PurgeConversation(ctx, convID, domain.PurgedBody)
	if err != nil || purged != 2 {
		t.Fatalf("purge = (%d, %v), want 2", purged, err)
	}
	transcript, _ = messages.Transcript(ctx, convID)
	for _, m := range transcript {
		if m.Body != domain.PurgedBody || len(m.RefSnapshot) != 0 {
			t.Fatalf("message %s kept its words: %q %s", m.ID, m.Body, m.RefSnapshot)
		}
	}
}

func containsMessage(messages []domain.Message, id string) bool {
	for _, m := range messages {
		if m.ID == id {
			return true
		}
	}
	return false
}

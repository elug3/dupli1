package livefeed

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/elug3/dupli1/support/pkg/ports"
)

func TestCustomerHearsOnlyOwnConversation(t *testing.T) {
	hub := NewHub()
	alice := hub.SubscribeCustomer("alice")
	bob := hub.SubscribeCustomer("bob")
	inbox := hub.SubscribeInbox()
	defer hub.Unsubscribe(alice)
	defer hub.Unsubscribe(bob)
	defer hub.Unsubscribe(inbox)

	hub.PublishLive(context.Background(), ports.LiveEvent{
		Type: ports.LiveMessage, InquiryID: "inq-1", ConversationID: "conv-1",
		CustomerID: "alice", Channel: "web", MessageID: "msg-1",
	})

	select {
	case frame := <-alice.C:
		if frame.Event != ports.LiveMessage {
			t.Fatalf("event = %q", frame.Event)
		}
		var body map[string]any
		_ = json.Unmarshal(frame.Data, &body)
		if body["message_id"] != "msg-1" {
			t.Fatalf("frame = %s", frame.Data)
		}
		if _, leaked := body["conversation_id"]; leaked {
			t.Fatalf("customer frame carries staff fields: %s", frame.Data)
		}
	default:
		t.Fatal("alice heard nothing")
	}
	select {
	case frame := <-bob.C:
		t.Fatalf("bob heard alice's conversation: %s", frame.Data)
	default:
	}
	select {
	case <-inbox.C:
	default:
		t.Fatal("inbox heard nothing")
	}
}

func TestTelegramEventReachesInboxOnly(t *testing.T) {
	hub := NewHub()
	shopper := hub.SubscribeCustomer("")
	inbox := hub.SubscribeInbox()

	hub.PublishLive(context.Background(), ports.LiveEvent{Type: ports.LiveInquiry, InquiryID: "inq-2", Channel: "telegram"})

	select {
	case <-shopper.C:
		t.Fatal("a stream with no customer id must hear nothing")
	default:
	}
	select {
	case <-inbox.C:
	default:
		t.Fatal("inbox heard nothing")
	}
}

func TestSlowSubscriberIsDropped(t *testing.T) {
	hub := NewHub()
	sub := hub.SubscribeInbox()
	for i := 0; i < subscriberQueue+1; i++ {
		hub.PublishLive(context.Background(), ports.LiveEvent{Type: ports.LiveMessage, InquiryID: "x"})
	}
	if hub.Subscribers() != 0 {
		t.Fatalf("subscribers = %d, want the slow one dropped", hub.Subscribers())
	}
	for range sub.C {
	}
	hub.Unsubscribe(sub) // safe after a drop
}

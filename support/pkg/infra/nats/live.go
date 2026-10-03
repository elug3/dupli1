package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/elug3/dupli1/shared/pkg/natsauth"
	"github.com/elug3/dupli1/shared/pkg/natspublisher"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// workerQueue is the queue group for work that must happen once across
// replicas (user.deleted). Live relays use no queue group: every replica must
// hear every change, since a stream may be held by any of them.
const workerQueue = "support-workers"

// LivePublisher sends chat stream changes over NATS, where every replica's
// Subscriber hands them to its own hub.
type LivePublisher struct {
	publisher *natspublisher.Publisher
	now       func() time.Time
}

func NewLivePublisher(publisher *natspublisher.Publisher) *LivePublisher {
	return &LivePublisher{publisher: publisher, now: time.Now}
}

// PublishLive is fire and forget: a lost frame costs a stream its freshness
// until the next change or reconnect, never a message.
func (p *LivePublisher) PublishLive(ctx context.Context, event ports.LiveEvent) {
	if p == nil || p.publisher == nil {
		return
	}
	subject := events.SupportInquiryUpdated
	if event.Type == ports.LiveMessage {
		subject = events.SupportMessageCreated
	}
	err := p.publisher.Publish(ctx, subject, events.SupportLive{
		InquiryID:      event.InquiryID,
		ConversationID: event.ConversationID,
		CustomerID:     event.CustomerID,
		Channel:        event.Channel,
		MessageID:      event.MessageID,
		Status:         event.Status,
		Occurred:       p.now().UTC(),
	})
	if err != nil {
		log.Printf("support live publish %s: %v", subject, err)
	}
}

// Subscriber listens for the support service's own live events and for
// account deletions.
type Subscriber struct {
	conn      *natsgo.Conn
	mu        sync.Mutex
	subs      []*natsgo.Subscription
	closeOnce sync.Once
}

func NewSubscriber(url string) (*Subscriber, error) {
	if url == "" {
		url = natsgo.DefaultURL
	}
	conn, err := natsgo.Connect(url, natsauth.ConnectOpts()...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	return &Subscriber{conn: conn}, nil
}

// RelayLive feeds every replica's hub from the bus. No queue group, on
// purpose.
func (s *Subscriber) RelayLive(sink ports.LivePublisher) error {
	for _, subject := range []string{events.SupportMessageCreated, events.SupportInquiryUpdated} {
		eventType := ports.LiveInquiry
		if subject == events.SupportMessageCreated {
			eventType = ports.LiveMessage
		}
		sub, err := s.conn.Subscribe(subject, func(msg *natsgo.Msg) {
			var payload events.SupportLive
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				log.Printf("support live: unreadable %s payload", msg.Subject)
				return
			}
			sink.PublishLive(context.Background(), ports.LiveEvent{
				Type:           eventType,
				InquiryID:      payload.InquiryID,
				ConversationID: payload.ConversationID,
				CustomerID:     payload.CustomerID,
				Channel:        payload.Channel,
				MessageID:      payload.MessageID,
				Status:         payload.Status,
			})
		})
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		s.track(sub)
	}
	return nil
}

// OnUserDeleted runs forget once per deleted account across replicas.
func (s *Subscriber) OnUserDeleted(ctx context.Context, forget func(ctx context.Context, customerID string) error) error {
	sub, err := s.conn.QueueSubscribe(events.UserDeleted, workerQueue, func(msg *natsgo.Msg) {
		var evt events.UserDeletedEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil || evt.UserID == "" {
			log.Printf("support %s: unreadable payload", events.UserDeleted)
			return
		}
		if err := forget(ctx, evt.UserID); err != nil {
			log.Printf("support forget deleted account: %v", err)
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", events.UserDeleted, err)
	}
	s.track(sub)
	return nil
}

func (s *Subscriber) track(sub *natsgo.Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, sub)
}

// Close drains subscriptions and the connection.
func (s *Subscriber) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		for _, sub := range s.subs {
			_ = sub.Unsubscribe()
		}
		s.mu.Unlock()
		if s.conn != nil {
			s.conn.Close()
		}
	})
}

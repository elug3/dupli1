package nats

import (
	"context"
	"fmt"
	"sync"

	natsgo "github.com/nats-io/nats.go"
	"github.com/elug3/schick/notification/pkg/ports"
)

// Subscriber listens to NATS subjects and dispatches messages to handlers.
type Subscriber struct {
	conn      *natsgo.Conn
	subs      []*natsgo.Subscription
	closeOnce sync.Once
}

// NewSubscriber connects to NATS and returns an event subscriber.
func NewSubscriber(url string, opts ...natsgo.Option) (*Subscriber, error) {
	if url == "" {
		url = natsgo.DefaultURL
	}

	conn, err := natsgo.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	return &Subscriber{conn: conn}, nil
}

// Subscribe registers a handler for a subject.
func (s *Subscriber) Subscribe(ctx context.Context, subject string, handler ports.MessageHandler) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("nats subscriber not initialized")
	}

	sub, err := s.conn.Subscribe(subject, func(msg *natsgo.Msg) {
		if handler == nil {
			return
		}
		_ = handler(ctx, msg.Subject, msg.Data)
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", subject, err)
	}

	s.subs = append(s.subs, sub)
	return nil
}

// Close drains and closes the NATS connection.
func (s *Subscriber) Close() {
	s.closeOnce.Do(func() {
		if s == nil {
			return
		}
		for _, sub := range s.subs {
			_ = sub.Unsubscribe()
		}
		if s.conn != nil {
			_ = s.conn.Drain()
			s.conn.Close()
		}
	})
}

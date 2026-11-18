package nats

import (
	"context"
	"fmt"
	"log"
	"sync"

	natsgo "github.com/nats-io/nats.go"

	"github.com/elug3/dupli1/shared/pkg/natsauth"
)

// QueueGroup shares payment's event consumers across replicas, so each order
// event is applied to the points ledger by one replica only.
const QueueGroup = "payment-workers"

// Handler handles one message.
type Handler func(ctx context.Context, subject string, data []byte) error

// Subscriber listens to NATS subjects in payment's queue group.
type Subscriber struct {
	conn      *natsgo.Conn
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

func (s *Subscriber) Subscribe(ctx context.Context, subject string, handler Handler) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("nats subscriber not initialized")
	}
	sub, err := s.conn.QueueSubscribe(subject, QueueGroup, func(msg *natsgo.Msg) {
		if err := handler(ctx, msg.Subject, msg.Data); err != nil {
			log.Printf("payment nats handler subject=%s error=%v", msg.Subject, err)
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", subject, err)
	}
	s.subs = append(s.subs, sub)
	return nil
}

func (s *Subscriber) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		for _, sub := range s.subs {
			_ = sub.Unsubscribe()
		}
		if s.conn != nil {
			s.conn.Close()
		}
	})
}

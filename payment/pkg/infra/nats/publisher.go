package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

type Publisher struct {
	conn *natsgo.Conn
}

func NewPublisher(url string) (*Publisher, error) {
	conn, err := natsgo.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	return &Publisher{conn: conn}, nil
}

func (p *Publisher) Publish(ctx context.Context, subject string, event any) error {
	if p == nil || p.conn == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := p.conn.Publish(subject, payload); err != nil {
		return err
	}
	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return p.conn.FlushWithContext(flushCtx)
}

func (p *Publisher) Close() {
	if p != nil && p.conn != nil {
		p.conn.Close()
	}
}

var _ ports.EventPublisher = (*Publisher)(nil)

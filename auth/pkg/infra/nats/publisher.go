package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

const defaultFlushTimeout = 5 * time.Second

// Publisher publishes JSON-encoded events to NATS subjects.
type Publisher struct {
	conn *natsgo.Conn
}

// NewPublisher connects to NATS and returns an event publisher.
func NewPublisher(url string, opts ...natsgo.Option) (*Publisher, error) {
	if url == "" {
		url = natsgo.DefaultURL
	}

	conn, err := natsgo.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	return &Publisher{conn: conn}, nil
}

// Publish marshals event as JSON and publishes it to subject.
func (p *Publisher) Publish(ctx context.Context, subject string, event any) error {
	if p == nil || p.conn == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := p.conn.Publish(subject, payload); err != nil {
		return fmt.Errorf("publish nats event: %w", err)
	}
	flushCtx, cancel := flushContext(ctx)
	defer cancel()
	if err := p.conn.FlushWithContext(flushCtx); err != nil {
		return fmt.Errorf("flush nats event: %w", err)
	}

	return nil
}

// flushContext returns a context suitable for FlushWithContext, which requires a deadline.
func flushContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultFlushTimeout)
}

// Close closes the NATS connection.
func (p *Publisher) Close() {
	if p != nil && p.conn != nil {
		p.conn.Close()
	}
}

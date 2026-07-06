package ports

import "context"

// MessageHandler processes a single NATS message payload.
type MessageHandler func(ctx context.Context, subject string, payload []byte) error

// EventSubscriber registers handlers for event subjects.
type EventSubscriber interface {
	Subscribe(ctx context.Context, subject string, handler MessageHandler) error
	Close()
}

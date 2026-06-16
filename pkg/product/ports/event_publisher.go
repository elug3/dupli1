package ports

import "context"

// EventPublisher publishes integration events for other services.
type EventPublisher interface {
	Publish(ctx context.Context, subject string, event any) error
}

package ports

import "context"

type EventPublisher interface {
	Publish(ctx context.Context, subject string, event any) error
}

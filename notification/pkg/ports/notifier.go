package ports

import "context"

// Notifier sends outbound messages to a messaging channel.
type Notifier interface {
	Send(ctx context.Context, chatID string, message string) error
}

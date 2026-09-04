// Package consumer wires NATS event payloads to profile service use cases.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/elug3/dupli1/shared/pkg/events"
)

// ProfileDeleter is the subset of *service.Service this consumer needs —
// declared locally so the consumer package doesn't import service for
// wiring purposes and stays trivially testable with a fake.
type ProfileDeleter interface {
	DeleteUserData(ctx context.Context, userID string) error
}

// HandleUserDeleted returns a ports.MessageHandler-shaped function that
// decodes an events.UserDeletedEvent payload and deletes the user's
// profile data. Registered on the events.UserDeleted subject.
func HandleUserDeleted(svc ProfileDeleter) func(ctx context.Context, subject string, payload []byte) error {
	return func(ctx context.Context, subject string, payload []byte) error {
		var evt events.UserDeletedEvent
		if err := json.Unmarshal(payload, &evt); err != nil {
			return fmt.Errorf("decode %s payload: %w", subject, err)
		}
		if evt.UserID == "" {
			return fmt.Errorf("%s payload missing user_id", subject)
		}
		if err := svc.DeleteUserData(ctx, evt.UserID); err != nil {
			return fmt.Errorf("delete user data for %s: %w", evt.UserID, err)
		}
		return nil
	}
}

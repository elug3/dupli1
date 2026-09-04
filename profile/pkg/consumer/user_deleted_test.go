package consumer_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elug3/dupli1/profile/pkg/consumer"
	"github.com/elug3/dupli1/profile/pkg/infra/memory"
	"github.com/elug3/dupli1/profile/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/google/uuid"
)

func TestHandleUserDeleted_DeletesProfileData(t *testing.T) {
	repo := memory.NewProfileRepository()
	svc := service.New(repo)
	ctx := context.Background()
	userID := uuid.New().String()

	name := "윤라희"
	if _, err := svc.PatchProfile(ctx, userID, service.ProfilePatch{DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAddress(ctx, userID, service.AddressInput{
		RecipientName: "A", RecipientPhone: "01011112222", PostalCode: "06194",
		AddressLine1: "One", City: "강남구", Province: "서울",
	}); err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(events.UserDeletedEvent{
		EventType: events.UserDeleted,
		UserID:    userID,
		Occurred:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := consumer.HandleUserDeleted(svc)
	if err := handler(ctx, events.UserDeleted, payload); err != nil {
		t.Fatalf("handler: %v", err)
	}

	view, err := svc.GetProfileView(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if view.DisplayName != "" || len(view.Addresses) != 0 {
		t.Fatalf("expected profile data deleted, got %+v", view)
	}
}

func TestHandleUserDeleted_InvalidPayload(t *testing.T) {
	svc := service.New(memory.NewProfileRepository())
	handler := consumer.HandleUserDeleted(svc)

	if err := handler(context.Background(), events.UserDeleted, []byte(`not-json`)); err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
	if err := handler(context.Background(), events.UserDeleted, []byte(`{}`)); err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

type failingDeleter struct{}

func (failingDeleter) DeleteUserData(context.Context, string) error {
	return context.DeadlineExceeded
}

func TestHandleUserDeleted_PropagatesServiceError(t *testing.T) {
	handler := consumer.HandleUserDeleted(failingDeleter{})
	payload, _ := json.Marshal(events.UserDeletedEvent{UserID: "user-1"})
	if err := handler(context.Background(), events.UserDeleted, payload); err == nil {
		t.Fatal("expected error propagated from service")
	}
}

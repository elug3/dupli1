package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/elug3/schick/pkg/auth/domain"
)

type fakeUserRepository struct {
	saved *domain.User
}

func (r *fakeUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, nil
}

func (r *fakeUserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	return nil, nil
}

func (r *fakeUserRepository) Save(ctx context.Context, u *domain.User) error {
	u.ID = "user-123"
	r.saved = u
	return nil
}

func (r *fakeUserRepository) Delete(ctx context.Context, id string) error {
	return nil
}

type fakeTokenGenerator struct{}

func (g fakeTokenGenerator) Generate(ctx context.Context, userID string) (string, error) {
	return "token", nil
}

func (g fakeTokenGenerator) Validate(ctx context.Context, token string) (string, error) {
	return "user-123", nil
}

type recordedEventPublisher struct {
	subject string
	event   any
}

func (p *recordedEventPublisher) Publish(ctx context.Context, subject string, event any) error {
	p.subject = subject
	p.event = event
	return nil
}

func TestRegisterPublishesUserRegisteredEvent(t *testing.T) {
	repo := &fakeUserRepository{}
	publisher := &recordedEventPublisher{}
	svc := NewService(repo, fakeTokenGenerator{}, publisher)

	user, err := svc.Register(context.Background(), "customer@example.com", "secret")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if repo.saved != user {
		t.Fatalf("Register did not save returned user")
	}
	if publisher.subject != userRegisteredSubject {
		t.Fatalf("published subject = %q, want %q", publisher.subject, userRegisteredSubject)
	}

	event, ok := publisher.event.(userRegisteredEvent)
	if !ok {
		t.Fatalf("published event type = %T, want userRegisteredEvent", publisher.event)
	}
	if event.UserID != "user-123" {
		t.Fatalf("event.UserID = %q, want user-123", event.UserID)
	}
	if event.Email != "customer@example.com" {
		t.Fatalf("event.Email = %q, want customer@example.com", event.Email)
	}
	if event.EventType != userRegisteredSubject {
		t.Fatalf("event.EventType = %q, want %q", event.EventType, userRegisteredSubject)
	}
	if event.Occurred.IsZero() {
		t.Fatalf("event.Occurred is zero")
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal event returned error: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("Unmarshal event returned error: %v", err)
	}
	if _, ok := fields["password"]; ok {
		t.Fatalf("user registered event includes password")
	}
}

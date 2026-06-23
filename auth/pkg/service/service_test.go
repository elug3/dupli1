package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/elug3/schick/auth/pkg/domain"
	"github.com/elug3/schick/auth/pkg/ports"
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

func (g fakeTokenGenerator) Generate(ctx context.Context, userID string, roles []string) (string, error) {
	return "token", nil
}

func (g fakeTokenGenerator) Validate(ctx context.Context, token string) (ports.Claims, error) {
	return ports.Claims{UserID: "user-123", Roles: []string{"customer"}}, nil
}

type capturingTokenGenerator struct {
	capturedUserID string
	capturedRoles  []string
}

func (g *capturingTokenGenerator) Generate(ctx context.Context, userID string, roles []string) (string, error) {
	g.capturedUserID = userID
	g.capturedRoles = append([]string(nil), roles...)
	return "token", nil
}

func (g *capturingTokenGenerator) Validate(ctx context.Context, token string) (ports.Claims, error) {
	return ports.Claims{UserID: g.capturedUserID, Roles: g.capturedRoles}, nil
}

type stubUserRepository struct {
	user *domain.User
}

func (r *stubUserRepository) FindByEmail(_ context.Context, _ string) (*domain.User, error) {
	return r.user, nil
}

func (r *stubUserRepository) FindByID(_ context.Context, _ string) (*domain.User, error) {
	return r.user, nil
}

func (r *stubUserRepository) Save(_ context.Context, u *domain.User) error {
	if u.ID == "" {
		u.ID = "user-999"
	}
	return nil
}

func (r *stubUserRepository) Delete(_ context.Context, _ string) error { return nil }

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

func TestLogin_ForwardsUserRolesToTokenGenerator(t *testing.T) {
	user := domain.NewUser("u-1", "user@example.com", "pass", "order_manager")
	repo := &stubUserRepository{user: user}
	gen := &capturingTokenGenerator{}
	svc := NewService(repo, gen)

	if _, err := svc.Login(context.Background(), "user@example.com", "pass"); err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if gen.capturedUserID != "u-1" {
		t.Fatalf("Generate userID = %q, want u-1", gen.capturedUserID)
	}
	if len(gen.capturedRoles) != 1 || gen.capturedRoles[0] != "order_manager" {
		t.Fatalf("Generate roles = %v, want [order_manager]", gen.capturedRoles)
	}
}

func TestRefresh_FetchesFreshRolesFromDB(t *testing.T) {
	user := domain.NewUser("u-2", "user@example.com", "pass", "admin")
	repo := &stubUserRepository{user: user}
	gen := &capturingTokenGenerator{}
	svc := NewService(repo, gen)

	// Validate on the fake returns the captured ID; pre-seed it.
	gen.capturedUserID = "u-2"

	if _, err := svc.Refresh(context.Background(), "any-token"); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if len(gen.capturedRoles) != 1 || gen.capturedRoles[0] != "admin" {
		t.Fatalf("Generate roles = %v, want [admin]", gen.capturedRoles)
	}
}

func TestRegisterAssignsCustomerRole(t *testing.T) {
	repo := &fakeUserRepository{}
	svc := NewService(repo, fakeTokenGenerator{})

	user, err := svc.Register(context.Background(), "customer@example.com", "secret")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "customer" {
		t.Fatalf("Register roles = %v, want [customer]", user.Roles)
	}
}

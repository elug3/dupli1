package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/elug3/schick/pkg/auth/autherrors"
	"github.com/elug3/schick/pkg/auth/domain"
	jwtgen "github.com/elug3/schick/pkg/auth/infra/jwt"
	"github.com/elug3/schick/pkg/auth/infra/memory"
	"github.com/google/uuid"
)

type fakeUserRepository struct {
	mu      sync.RWMutex
	byID    map[uuid.UUID]*domain.User
	byEmail map[string]*domain.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{
		byID:    make(map[uuid.UUID]*domain.User),
		byEmail: make(map[string]*domain.User),
	}
}

func (r *fakeUserRepository) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byEmail[email], nil
}

func (r *fakeUserRepository) FindByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byID[id], nil
}

func (r *fakeUserRepository) ListUsers(_ context.Context) ([]*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]*domain.User, 0, len(r.byID))
	for _, user := range r.byID {
		users = append(users, user)
	}
	return users, nil
}

func (r *fakeUserRepository) Save(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.byEmail[u.Email]; ok && existing.ID != u.ID {
		return autherrors.ErrUserAlreadyExists
	}
	if old, ok := r.byID[u.ID]; ok && old.Email != u.Email {
		delete(r.byEmail, old.Email)
	}
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u
	return nil
}

func (r *fakeUserRepository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if user, ok := r.byID[id]; ok {
		delete(r.byEmail, user.Email)
		delete(r.byID, id)
	}
	return nil
}

type recordedEventPublisher struct {
	calls   int
	subject string
	event   any
}

func (p *recordedEventPublisher) Publish(_ context.Context, subject string, event any) error {
	p.calls++
	p.subject = subject
	p.event = event
	return nil
}

func newTestService() (*Service, *fakeUserRepository, *recordedEventPublisher) {
	repo := newFakeUserRepository()
	publisher := &recordedEventPublisher{}
	accessGen := jwtgen.NewTokenGenerator("test-signing-secret", int64((15 * time.Minute).Seconds()))
	refreshGen := jwtgen.NewTokenGenerator("test-signing-secret", int64((24 * time.Hour).Seconds()))
	sessions := memory.NewSessionStore()
	svc := NewService(repo, accessGen, refreshGen, sessions, 24*time.Hour, publisher)
	return svc, repo, publisher
}

func TestRegisterPublishesUserRegisteredEvent(t *testing.T) {
	svc, repo, publisher := newTestService()

	user, err := svc.Register(context.Background(), "customer@example.com", "supersecret")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	saved, err := repo.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if saved != user {
		t.Fatalf("Register did not save returned user")
	}
	if publisher.calls != 1 {
		t.Fatalf("publisher calls = %d, want 1", publisher.calls)
	}
	if publisher.subject != userRegisteredSubject {
		t.Fatalf("published subject = %q, want %q", publisher.subject, userRegisteredSubject)
	}

	event, ok := publisher.event.(userRegisteredEvent)
	if !ok {
		t.Fatalf("published event type = %T, want userRegisteredEvent", publisher.event)
	}
	if event.UserID != user.ID {
		t.Fatalf("event.UserID = %q, want %q", event.UserID, user.ID)
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

func TestLoginIssuesTokensAndMeReturnsUser(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	user, err := svc.Register(ctx, "login@example.com", "supersecret")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	tokens, err := svc.Login(ctx, "login@example.com", "supersecret")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("access token is empty")
	}
	if tokens.RefreshToken == "" {
		t.Fatal("refresh token is empty")
	}

	got, err := svc.Me(ctx, tokens.AccessToken)
	if err != nil {
		t.Fatalf("Me returned error: %v", err)
	}
	if got.ID != user.ID {
		t.Fatalf("Me returned user ID %s, want %s", got.ID, user.ID)
	}

	if _, err := svc.Login(ctx, "login@example.com", "wrongpassword"); !errors.Is(err, autherrors.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Me(ctx, tokens.RefreshToken); !errors.Is(err, autherrors.ErrInvalidToken) {
		t.Fatalf("refresh token accepted as access token: %v", err)
	}
}

func TestRefreshRotatesRefreshToken(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "refresh@example.com", "supersecret"); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	tokens, err := svc.Login(ctx, "refresh@example.com", "supersecret")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	rotated, err := svc.Refresh(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if rotated.AccessToken == "" {
		t.Fatal("rotated access token is empty")
	}
	if rotated.RefreshToken == "" {
		t.Fatal("rotated refresh token is empty")
	}
	if rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	if _, err := svc.Refresh(ctx, tokens.RefreshToken); !errors.Is(err, autherrors.ErrInvalidToken) {
		t.Fatalf("old refresh token error = %v, want ErrInvalidToken", err)
	}
	if _, err := svc.Refresh(ctx, rotated.AccessToken); !errors.Is(err, autherrors.ErrInvalidToken) {
		t.Fatalf("access token accepted as refresh token: %v", err)
	}
}

func TestLogoutInvalidatesRefreshToken(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "logout@example.com", "supersecret"); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	tokens, err := svc.Login(ctx, "logout@example.com", "supersecret")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if err := svc.Logout(ctx, tokens.RefreshToken); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if _, err := svc.Refresh(ctx, tokens.RefreshToken); !errors.Is(err, autherrors.ErrInvalidToken) {
		t.Fatalf("logged out refresh token error = %v, want ErrInvalidToken", err)
	}
}

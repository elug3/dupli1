package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
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

func (r *fakeUserRepository) ListAll(ctx context.Context) ([]*domain.User, error) {
	return nil, nil
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

func (r *stubUserRepository) ListAll(_ context.Context) ([]*domain.User, error) { return nil, nil }

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
	svc := NewService(repo, fakeTokenGenerator{}, WithEventPublisher(publisher))

	user, err := svc.Register(context.Background(), "customer@example.com", "supersecret", domain.AccountTypeCustomer)
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
	if event.AccountType != domain.AccountTypeCustomer {
		t.Fatalf("event.AccountType = %q, want %q", event.AccountType, domain.AccountTypeCustomer)
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
	user, _ := domain.NewUser("u-1", "user@example.com", "pass", domain.AccountTypeService, "order_manager")
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
	user, _ := domain.NewUser("u-2", "user@example.com", "pass", domain.AccountTypeAdmin, "admin")
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

func TestRegisterRejectsInvalidAccountType(t *testing.T) {
	svc := NewService(&fakeUserRepository{}, fakeTokenGenerator{})

	if _, err := svc.Register(context.Background(), "customer@example.com", "supersecret", "staff"); !errors.Is(err, autherrors.ErrInvalidAccountType) {
		t.Fatalf("got %v, want ErrInvalidAccountType", err)
	}
}

func TestRegisterAssignsCustomerRole(t *testing.T) {
	repo := &fakeUserRepository{}
	svc := NewService(repo, fakeTokenGenerator{})

	user, err := svc.Register(context.Background(), "customer@example.com", "supersecret", domain.AccountTypeCustomer)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "customer" {
		t.Fatalf("Register roles = %v, want [customer]", user.Roles)
	}
	if user.AccountType != domain.AccountTypeCustomer {
		t.Fatalf("Register account_type = %q, want %q", user.AccountType, domain.AccountTypeCustomer)
	}
}

type mutableUserRepository struct {
	user *domain.User
}

func (r *mutableUserRepository) FindByEmail(_ context.Context, _ string) (*domain.User, error) {
	return r.user, nil
}

func (r *mutableUserRepository) FindByID(_ context.Context, _ string) (*domain.User, error) {
	return r.user, nil
}

func (r *mutableUserRepository) Save(_ context.Context, u *domain.User) error {
	r.user = u
	return nil
}

func (r *mutableUserRepository) Delete(_ context.Context, _ string) error { return nil }

func (r *mutableUserRepository) ListAll(_ context.Context) ([]*domain.User, error) { return nil, nil }

func TestLogin_LocksAccountAfterMaxFailedAttempts(t *testing.T) {
	user, _ := domain.NewUser("u-lock", "locked@example.com", "correct-pass", domain.AccountTypeCustomer, "customer")
	repo := &mutableUserRepository{user: user}
	svc := NewService(repo, fakeTokenGenerator{})

	for i := 0; i < maxFailedAttempts; i++ {
		if _, err := svc.Login(context.Background(), "locked@example.com", "wrong"); err == nil {
			t.Fatalf("attempt %d: expected error", i+1)
		}
	}
	if !repo.user.IsLocked() {
		t.Fatal("account should be locked after max failed attempts")
	}

	if _, err := svc.Login(context.Background(), "locked@example.com", "correct-pass"); !errors.Is(err, autherrors.ErrAccountLocked) {
		t.Fatalf("locked login: got %v, want ErrAccountLocked", err)
	}
}

func TestLogin_RejectsDeactivatedAccount(t *testing.T) {
	user, _ := domain.NewUser("u-off", "off@example.com", "pass", domain.AccountTypeCustomer, "customer")
	user.SetActive(false)
	repo := &stubUserRepository{user: user}
	svc := NewService(repo, fakeTokenGenerator{})

	if _, err := svc.Login(context.Background(), "off@example.com", "pass"); !errors.Is(err, autherrors.ErrAccountDeactivated) {
		t.Fatalf("got %v, want ErrAccountDeactivated", err)
	}
}

func TestRefresh_RejectsDeactivatedAccount(t *testing.T) {
	user, _ := domain.NewUser("u-off", "off@example.com", "pass", domain.AccountTypeCustomer, "customer")
	user.SetActive(false)
	repo := &stubUserRepository{user: user}
	gen := &capturingTokenGenerator{capturedUserID: "u-off"}
	svc := NewService(repo, fakeTokenGenerator{}, WithRefreshTokenGen(gen, time.Hour))

	if _, err := svc.Refresh(context.Background(), "refresh-token"); !errors.Is(err, autherrors.ErrAccountDeactivated) {
		t.Fatalf("got %v, want ErrAccountDeactivated", err)
	}
}

type memorySessionStore struct {
	entries map[string]string
}

func (s *memorySessionStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	if s.entries == nil {
		s.entries = make(map[string]string)
	}
	s.entries[key] = value
	return nil
}

func (s *memorySessionStore) Get(_ context.Context, key string) (string, error) {
	if s.entries == nil {
		return "", ports.ErrSessionNotFound
	}
	v, ok := s.entries[key]
	if !ok {
		return "", ports.ErrSessionNotFound
	}
	return v, nil
}

func (s *memorySessionStore) Delete(_ context.Context, key string) error {
	if s.entries != nil {
		delete(s.entries, key)
	}
	return nil
}

func TestLogout_RevokesRefreshSession(t *testing.T) {
	user, _ := domain.NewUser("u-1", "user@example.com", "pass", domain.AccountTypeCustomer, "customer")
	repo := &stubUserRepository{user: user}
	refreshGen := &capturingTokenGenerator{}
	accessGen := fakeTokenGenerator{}
	sessions := &memorySessionStore{}
	svc := NewService(
		repo,
		accessGen,
		WithRefreshTokenGen(refreshGen, time.Hour),
		WithSessionStore(sessions),
	)

	refreshToken, err := svc.Login(context.Background(), "user@example.com", "pass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, ok := sessions.entries[refreshToken]; !ok {
		t.Fatal("refresh token was not stored in session store")
	}

	if err := svc.Logout(context.Background(), refreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, ok := sessions.entries[refreshToken]; ok {
		t.Fatal("refresh token should be removed after logout")
	}

	if _, err := svc.Refresh(context.Background(), refreshToken); !errors.Is(err, autherrors.ErrInvalidToken) {
		t.Fatalf("Refresh after logout: got %v, want ErrInvalidToken", err)
	}
}

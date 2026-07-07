package bootstrap

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/rs/zerolog"
)

type seedFakeRepo struct {
	byEmail map[string]*domain.User
}

func newSeedFakeRepo() *seedFakeRepo {
	return &seedFakeRepo{byEmail: make(map[string]*domain.User)}
}

func (r *seedFakeRepo) Save(_ context.Context, u *domain.User) error {
	r.byEmail[u.Email] = u
	return nil
}

func (r *seedFakeRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	return r.byEmail[email], nil
}

func (r *seedFakeRepo) FindByID(context.Context, string) (*domain.User, error) {
	return nil, nil
}

func (r *seedFakeRepo) ListAll(context.Context) ([]*domain.User, error) {
	return nil, nil
}

func (r *seedFakeRepo) Delete(context.Context, string) error {
	return nil
}

var _ ports.UserRepository = (*seedFakeRepo)(nil)

func TestSeedWebServiceAccount_CreatesCustomerRegistrar(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := Config{
		WebServiceEmail:    "dupli1-web@internal.dupli1",
		WebServicePassword: "service-secret",
		Logger:             zerolog.Nop(),
	}

	if err := seedWebServiceAccount(context.Background(), cfg, repo); err != nil {
		t.Fatalf("seedWebServiceAccount: %v", err)
	}

	u := repo.byEmail["dupli1-web@internal.dupli1"]
	if u == nil {
		t.Fatal("service account was not created")
	}
	if !u.HasPermission(permissions.UserCreate) {
		t.Fatalf("permissions = %v, want [%s]", u.Permissions, permissions.UserCreate)
	}
	if !u.ValidatePassword("service-secret") {
		t.Fatal("stored password does not match configured password")
	}
}

func TestSeedWebServiceAccount_Idempotent(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := Config{
		WebServiceEmail:    "dupli1-web@internal.dupli1",
		WebServicePassword: "service-secret",
		Logger:             zerolog.Nop(),
	}

	if err := seedWebServiceAccount(context.Background(), cfg, repo); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	firstID := repo.byEmail["dupli1-web@internal.dupli1"].ID

	if err := seedWebServiceAccount(context.Background(), cfg, repo); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if got := repo.byEmail["dupli1-web@internal.dupli1"].ID; got != firstID {
		t.Fatalf("second seed changed user id: %s -> %s", firstID, got)
	}
}

func TestSeedWebServiceAccount_SkipsWhenEmailEmpty(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := Config{
		WebServicePassword: "service-secret",
		Logger:             zerolog.Nop(),
	}

	if err := seedWebServiceAccount(context.Background(), cfg, repo); err != nil {
		t.Fatalf("seedWebServiceAccount: %v", err)
	}
	if len(repo.byEmail) != 0 {
		t.Fatalf("expected no users, got %d", len(repo.byEmail))
	}
}

func TestSeedWebServiceAccount_RequiresPassword(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := Config{
		WebServiceEmail: "dupli1-web@internal.dupli1",
		Logger:          zerolog.Nop(),
	}

	if err := seedWebServiceAccount(context.Background(), cfg, repo); err == nil {
		t.Fatal("expected error when password is missing")
	}
}

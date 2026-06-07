package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/elug3/schick/pkg/auth/autherrors"
	"github.com/elug3/schick/pkg/auth/domain"
	"github.com/elug3/schick/pkg/auth/infra/postgres"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

var repo *postgres.UserRepository

func TestMain(m *testing.M) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		// Skip all DB tests when no database is configured.
		os.Exit(0)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic("open postgres: " + err.Error())
	}
	if err := db.Ping(); err != nil {
		panic("ping postgres: " + err.Error())
	}

	ctx := context.Background()
	repo, err = postgres.NewUserRepository(ctx, db)
	if err != nil {
		panic("init repo: " + err.Error())
	}

	code := m.Run()

	// Clean up the table after all tests.
	_, _ = db.Exec("DROP TABLE IF EXISTS users")
	db.Close()

	os.Exit(code)
}

func newTestUser(t *testing.T) *domain.User {
	t.Helper()
	u := domain.NewUser("test+" + uuid.New().String() + "@example.com")
	if err := u.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	return u
}

func TestSave_And_FindByEmail(t *testing.T) {
	ctx := context.Background()
	u := newTestUser(t)

	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByEmail(ctx, u.Email)
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.ID != u.ID {
		t.Errorf("ID: got %v, want %v", got.ID, u.ID)
	}
	if got.Email != u.Email {
		t.Errorf("Email: got %q, want %q", got.Email, u.Email)
	}
}

func TestFindByEmail_NotFound(t *testing.T) {
	ctx := context.Background()
	got, err := repo.FindByEmail(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSave_And_FindByID(t *testing.T) {
	ctx := context.Background()
	u := newTestUser(t)

	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.ID != u.ID {
		t.Errorf("ID: got %v, want %v", got.ID, u.ID)
	}
}

func TestFindByID_NotFound(t *testing.T) {
	ctx := context.Background()
	got, err := repo.FindByID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSave_UpdateExisting(t *testing.T) {
	ctx := context.Background()
	u := newTestUser(t)

	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	// Change email and re-save by same ID.
	newEmail := "updated+" + uuid.New().String() + "@example.com"
	u.Email = newEmail
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("update Save: %v", err)
	}

	got, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Email != newEmail {
		t.Errorf("Email: got %q, want %q", got.Email, newEmail)
	}
}

func TestSave_DuplicateEmail(t *testing.T) {
	ctx := context.Background()
	u1 := newTestUser(t)
	if err := repo.Save(ctx, u1); err != nil {
		t.Fatalf("Save u1: %v", err)
	}

	u2 := domain.NewUser(u1.Email) // same email, different ID
	if err := u2.SetPassword("password"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	err := repo.Save(ctx, u2)
	if err != autherrors.ErrUserAlreadyExists {
		t.Fatalf("expected ErrUserAlreadyExists, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	u := newTestUser(t)

	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindByID after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}

func TestDelete_NonExistent(t *testing.T) {
	ctx := context.Background()
	// Deleting a row that does not exist must not error.
	if err := repo.Delete(ctx, uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPasswordRoundtrip(t *testing.T) {
	ctx := context.Background()
	u := newTestUser(t)

	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByEmail(ctx, u.Email)
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}

	if !got.ValidatePassword("hunter2") {
		t.Error("password validation failed after round-trip")
	}
	if got.ValidatePassword("wrongpassword") {
		t.Error("wrong password should not validate")
	}
}

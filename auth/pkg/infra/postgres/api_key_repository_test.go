package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/infra/postgres"
	"github.com/google/uuid"
)

func newAPIKeyRepo(t *testing.T) (*postgres.APIKeyRepository, *sql.DB) {
	t.Helper()
	requirePostgres(t)
	db, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return postgres.NewAPIKeyRepository(db), db
}

func saveKey(t *testing.T, keys *postgres.APIKeyRepository, userID string, mutate func(*domain.APIKey)) (*domain.APIKey, string) {
	t.Helper()
	plaintext, _ := domain.GenerateAPIKey(domain.APIKeyEnvTest)
	k := &domain.APIKey{
		ID: uuid.New().String(), UserID: userID, Name: "k", Prefix: domain.APIKeyPrefix(plaintext),
		KeyHash: domain.HashAPIKey(plaintext), Source: domain.APIKeySourceAPI,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if mutate != nil {
		mutate(k)
	}
	if err := keys.Save(context.Background(), k); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return k, plaintext
}

func TestAPIKeyRepository_RoundTrip(t *testing.T) {
	keys, _ := newAPIKeyRepo(t)
	ctx := t.Context()
	u := newTestUser(t)
	u.AccountType = domain.AccountTypeService
	if err := repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	k, plaintext := saveKey(t, keys, u.ID, func(k *domain.APIKey) {
		k.Permissions = []string{"order.ship"}
		k.ExpiresAt = &exp
		k.CreatedBy = "owner-id"
	})

	got, err := keys.FindByHash(ctx, domain.HashAPIKey(plaintext))
	if err != nil || got == nil {
		t.Fatalf("FindByHash: %v, %v", got, err)
	}
	if got.ID != k.ID || got.UserID != u.ID || got.Prefix != k.Prefix || len(got.Permissions) != 1 ||
		got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) || got.CreatedBy != "owner-id" || got.RevokedAt != nil || got.LastUsedAt != nil {
		t.Fatalf("round trip: %+v", got)
	}

	used := time.Now().UTC().Truncate(time.Microsecond)
	if err := keys.TouchLastUsed(ctx, k.ID, used); err != nil {
		t.Fatal(err)
	}
	got.RevokedAt = &used
	if err := keys.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, _ := keys.FindByID(ctx, k.ID)
	if again.RevokedAt == nil || again.LastUsedAt == nil || !again.LastUsedAt.Equal(used) {
		t.Fatalf("after revoke/touch: %+v", again)
	}

	_, _ = saveKey(t, keys, u.ID, nil)
	list, err := keys.ListByUser(ctx, u.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListByUser: %d keys, %v", len(list), err)
	}
	if missing, _ := keys.FindByHash(ctx, "0000"); missing != nil {
		t.Fatal("unknown hash found")
	}
}

func TestAPIKeyRepository_HashIsUniqueAndDeleteCascades(t *testing.T) {
	keys, _ := newAPIKeyRepo(t)
	ctx := t.Context()
	u := newTestUser(t)
	if err := repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}
	k, _ := saveKey(t, keys, u.ID, nil)
	dup := *k
	dup.ID = uuid.New().String()
	if err := keys.Save(ctx, &dup); err == nil {
		t.Fatal("two rows with one key hash must be refused")
	}
	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if gone, _ := keys.FindByID(ctx, k.ID); gone != nil {
		t.Fatal("deleting the account must delete its keys")
	}
}

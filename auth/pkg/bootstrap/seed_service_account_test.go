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

func testKey(t *testing.T) string {
	t.Helper()
	k, err := domain.GenerateAPIKey(domain.APIKeyEnvTest)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func webCfg(t *testing.T) Config {
	return Config{
		WebServiceEmail:  "dupli1-web@internal.dupli1",
		WebServiceAPIKey: testKey(t),
		Logger:           zerolog.Nop(),
	}
}

func TestSeedWebServiceAccount_CreatesKeyOnlyRegistrar(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := webCfg(t)
	keys := &recordingKeySyncer{}
	if err := seedWebServiceAccount(t.Context(), cfg, repo, keys); err != nil {
		t.Fatalf("seedWebServiceAccount: %v", err)
	}
	u := repo.byEmail["dupli1-web@internal.dupli1"]
	if u == nil {
		t.Fatal("service account was not created")
	}
	if !u.HasPermission(permissions.UserCreate) || u.AccountType != domain.AccountTypeService || u.ServiceName != "dupli1-web" {
		t.Fatalf("account = %+v", u)
	}
	if !u.PasswordRetired() {
		t.Fatal("a service account must have no password")
	}
	if len(keys.calls) != 1 || keys.calls[0] != u.ID+"="+cfg.WebServiceAPIKey {
		t.Fatalf("key sync calls = %v", keys.calls)
	}
}

func TestSeedWebServiceAccount_Idempotent(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := webCfg(t)
	if err := seedWebServiceAccount(t.Context(), cfg, repo, nil); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	first := repo.byEmail["dupli1-web@internal.dupli1"].ID
	if err := seedWebServiceAccount(t.Context(), cfg, repo, nil); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if got := repo.byEmail["dupli1-web@internal.dupli1"].ID; got != first {
		t.Fatalf("user id changed: %s -> %s", first, got)
	}
}

func TestSeedWebServiceAccount_SyncsAccountAndDropsPassword(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := webCfg(t)
	if err := seedWebServiceAccount(t.Context(), cfg, repo, nil); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	// Drift: permissions cleared, deactivated, wrong type, and a password set
	// (as an account created before keys would have).
	u := repo.byEmail["dupli1-web@internal.dupli1"]
	u.SetPermissions(nil)
	u.SetActive(false)
	u.AccountType = domain.AccountTypeCustomer
	_ = u.UpdatePassword("old-password")

	if err := seedWebServiceAccount(t.Context(), cfg, repo, nil); err != nil {
		t.Fatalf("sync seed: %v", err)
	}
	got := repo.byEmail["dupli1-web@internal.dupli1"]
	if got.ID != u.ID || !got.IsActive || got.AccountType != domain.AccountTypeService {
		t.Fatalf("account = %+v", got)
	}
	if !got.HasPermission(permissions.UserCreate) || len(got.Permissions) != 1 {
		t.Fatalf("permissions = %v", got.Permissions)
	}
	if !got.PasswordRetired() || got.ValidatePassword("old-password") {
		t.Fatal("the old password must stop working")
	}
}

func TestSeedWebServiceAccount_SkipsWhenEmailEmpty(t *testing.T) {
	repo := newSeedFakeRepo()
	if err := seedWebServiceAccount(t.Context(), Config{WebServiceAPIKey: testKey(t), Logger: zerolog.Nop()}, repo, nil); err != nil {
		t.Fatalf("seedWebServiceAccount: %v", err)
	}
	if len(repo.byEmail) != 0 {
		t.Fatalf("expected no users, got %d", len(repo.byEmail))
	}
}

func TestSeedWebServiceAccount_RequiresAPIKey(t *testing.T) {
	cfg := Config{
		WebServiceEmail:    "dupli1-web@internal.dupli1",
		WebServicePassword: "service-secret", // no longer enough
		Logger:             zerolog.Nop(),
	}
	if err := seedWebServiceAccount(t.Context(), cfg, newSeedFakeRepo(), nil); err == nil {
		t.Fatal("expected an error without an API key")
	}
}

func TestSeedOrderServiceAccount_CreatesAndSyncs(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := Config{
		OrderServiceEmail:  "dupli1-order@order.dupli1.com",
		OrderServiceAPIKey: testKey(t),
		Logger:             zerolog.Nop(),
	}
	if err := seedOrderServiceAccount(t.Context(), cfg, repo, nil); err != nil {
		t.Fatalf("seedOrderServiceAccount: %v", err)
	}
	u := repo.byEmail["dupli1-order@order.dupli1.com"]
	if u == nil || !hasExactPermissions(u, orderServicePermissions) || u.ServiceName != "dupli1-order" || !u.PasswordRetired() {
		t.Fatalf("order service account = %+v", u)
	}
	firstID := u.ID
	u.SetPermissions(nil)
	if err := seedOrderServiceAccount(t.Context(), cfg, repo, nil); err != nil {
		t.Fatalf("sync seed: %v", err)
	}
	got := repo.byEmail["dupli1-order@order.dupli1.com"]
	if got.ID != firstID || !hasExactPermissions(got, orderServicePermissions) {
		t.Fatalf("after sync: id %s -> %s, permissions %v", firstID, got.ID, got.Permissions)
	}
}

type recordingKeySyncer struct {
	calls []string // userID=plaintext
	err   error
}

func (s *recordingKeySyncer) SyncEnvAPIKey(_ context.Context, userID, plaintext string) (string, error) {
	s.calls = append(s.calls, userID+"="+plaintext)
	return "", s.err
}

func TestSeedOrderServiceAccount_PasswordEnvIsIgnored(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := Config{
		OrderServiceEmail:    "dupli1-order@order.dupli1.com",
		OrderServicePassword: "order-secret",
		OrderServiceAPIKey:   testKey(t),
		Logger:               zerolog.Nop(),
	}
	if err := seedOrderServiceAccount(t.Context(), cfg, repo, &recordingKeySyncer{}); err != nil {
		t.Fatal(err)
	}
	if u := repo.byEmail[cfg.OrderServiceEmail]; !u.PasswordRetired() || u.ValidatePassword("order-secret") {
		t.Fatal("a leftover *_SERVICE_PASSWORD must not become a password")
	}
}

func TestSeedServiceAccount_KeySyncFailureStopsBoot(t *testing.T) {
	repo := newSeedFakeRepo()
	cfg := webCfg(t)
	keys := &recordingKeySyncer{err: context.DeadlineExceeded}
	if err := seedWebServiceAccount(t.Context(), cfg, repo, keys); err == nil {
		t.Fatal("a failed key sync must fail the boot, not leave a stale key working")
	}
	if len(keys.calls) != 1 {
		t.Fatalf("key sync calls = %v", keys.calls)
	}
}

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/infra/memory"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

type mapUserRepo struct{ users map[string]*domain.User }

func (r *mapUserRepo) FindByEmail(context.Context, string) (*domain.User, error) { return nil, nil }
func (r *mapUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	return r.users[id], nil
}
func (r *mapUserRepo) ListAll(context.Context) ([]*domain.User, error) { return nil, nil }
func (r *mapUserRepo) Save(_ context.Context, u *domain.User) error    { r.users[u.ID] = u; return nil }
func (r *mapUserRepo) Delete(context.Context, string) error            { return nil }

type apiKeyFixture struct {
	svc   *Service
	gen   *capturingTokenGenerator
	keys  *memory.APIKeyRepository
	users *mapUserRepo
	order *domain.User
	now   time.Time
}

func newAPIKeyFixture(t *testing.T) *apiKeyFixture {
	t.Helper()
	order, _ := domain.NewUser("svc-order", "order@example.com", "pw-unused", domain.AccountTypeService,
		permissions.OrderShip, permissions.PromotionRedeem, permissions.InventoryReservationManage)
	order.ServiceName = "dupli1-order"
	manager, _ := domain.NewUser("mgr", "mgr@example.com", "pw-unused", domain.AccountTypeManager, permissions.OrderShip)
	f := &apiKeyFixture{
		gen:   &capturingTokenGenerator{},
		keys:  memory.NewAPIKeyRepository(),
		users: &mapUserRepo{users: map[string]*domain.User{order.ID: order, manager.ID: manager}},
		order: order,
		now:   time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}
	f.svc = NewService(f.users, f.gen, WithAPIKeyRepo(f.keys), WithAccessTokenTTL(15*time.Minute), WithAPIKeyEnv(domain.APIKeyEnvTest))
	f.svc.now = func() time.Time { return f.now }
	return f
}

func TestExchangeAPIKey_MintsServiceIdentity(t *testing.T) {
	f := newAPIKeyFixture(t)
	key, plaintext, err := f.svc.CreateAPIKey(t.Context(), f.order.ID, "checkout", []string{permissions.PromotionRedeem}, 0, "owner")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	res, err := f.svc.ExchangeAPIKey(t.Context(), plaintext)
	if err != nil {
		t.Fatalf("ExchangeAPIKey: %v", err)
	}
	if res.AccessToken == "" || res.ExpiresIn != 15*time.Minute {
		t.Fatalf("result = %+v", res)
	}
	id := f.gen.capturedIdentity
	if id.AccountType != "service" || id.ServiceName != "dupli1-order" || id.APIKeyID != key.ID || id.Email != "order@example.com" {
		t.Fatalf("identity = %+v; internal APIs need service_name and akid", id)
	}
	if got := f.gen.capturedPermissions; len(got) != 1 || got[0] != permissions.PromotionRedeem {
		t.Fatalf("permissions = %v, want the key's scope", got)
	}
}

func TestExchangeAPIKey_Rejections(t *testing.T) {
	f := newAPIKeyFixture(t)
	check := func(name, plaintext string, want domain.APIKeyRejection) {
		t.Helper()
		res, err := f.svc.ExchangeAPIKey(t.Context(), plaintext)
		if !errors.Is(err, autherrors.ErrInvalidAPIKey) || res.Reason != want || res.AccessToken != "" {
			t.Errorf("%s: err=%v reason=%q, want ErrInvalidAPIKey/%q", name, err, res.Reason, want)
		}
	}
	check("garbage", "not-a-key", domain.APIKeyUnknown)
	unknown, _ := domain.GenerateAPIKey(domain.APIKeyEnvTest)
	check("unknown", unknown, domain.APIKeyUnknown)

	revoked, p, _ := f.svc.CreateAPIKey(t.Context(), f.order.ID, "r", nil, 0, "owner")
	if _, err := f.svc.RevokeAPIKey(t.Context(), revoked.ID); err != nil {
		t.Fatal(err)
	}
	check("revoked", p, domain.APIKeyRevoked)

	_, p, _ = f.svc.CreateAPIKey(t.Context(), f.order.ID, "e", nil, time.Hour, "owner")
	f.now = f.now.Add(time.Hour)
	check("expired", p, domain.APIKeyExpired)

	_, p, _ = f.svc.CreateAPIKey(t.Context(), f.order.ID, "i", nil, 0, "owner")
	f.order.IsActive = false
	check("inactive account", p, domain.APIKeyInactive)
	f.order.IsActive = true

	// A key row pointing at a person (only possible by hand) still refuses.
	_ = f.keys.Save(t.Context(), &domain.APIKey{ID: "k-mgr", UserID: "mgr", KeyHash: domain.HashAPIKey(unknown)})
	check("person's account", unknown, domain.APIKeyNotService)
}

func TestExchangeAPIKey_IgnoresPasswordLockout(t *testing.T) {
	f := newAPIKeyFixture(t)
	_, p, _ := f.svc.CreateAPIKey(t.Context(), f.order.ID, "k", nil, 0, "owner")
	f.order.Lock()
	if _, err := f.svc.ExchangeAPIKey(t.Context(), p); err != nil {
		t.Fatalf("a password lockout must not take the service down: %v", err)
	}
}

func TestExchangeAPIKey_ThrottlesLastUsed(t *testing.T) {
	f := newAPIKeyFixture(t)
	key, p, _ := f.svc.CreateAPIKey(t.Context(), f.order.ID, "k", nil, 0, "owner")
	lastUsed := func() time.Time {
		k, _ := f.keys.FindByID(t.Context(), key.ID)
		if k.LastUsedAt == nil {
			return time.Time{}
		}
		return *k.LastUsedAt
	}
	first := f.now
	_, _ = f.svc.ExchangeAPIKey(t.Context(), p)
	f.now = f.now.Add(30 * time.Second)
	_, _ = f.svc.ExchangeAPIKey(t.Context(), p)
	if !lastUsed().Equal(first) {
		t.Fatalf("written again within the interval: %v", lastUsed())
	}
	f.now = f.now.Add(31 * time.Second)
	_, _ = f.svc.ExchangeAPIKey(t.Context(), p)
	if !lastUsed().Equal(f.now) {
		t.Fatalf("not written after the interval: %v", lastUsed())
	}
}

func TestCreateAPIKey_Validation(t *testing.T) {
	f := newAPIKeyFixture(t)
	cases := []struct {
		name, user, keyName string
		scope               []string
		ttl                 time.Duration
		want                error
	}{
		{"no name", f.order.ID, " ", nil, 0, autherrors.ErrInvalidAPIKeyRequest},
		{"negative ttl", f.order.ID, "k", nil, -time.Hour, autherrors.ErrInvalidAPIKeyRequest},
		{"unknown permission", f.order.ID, "k", []string{"nope.nope"}, 0, autherrors.ErrInvalidPermission},
		{"beyond the account", f.order.ID, "k", []string{permissions.PaymentCancel}, 0, autherrors.ErrScopeExceedsAccount},
		{"person's account", "mgr", "k", nil, 0, autherrors.ErrNotServiceAccount},
		{"no such account", "ghost", "k", nil, 0, autherrors.ErrUserNotFound},
	}
	for _, tc := range cases {
		if _, _, err := f.svc.CreateAPIKey(t.Context(), tc.user, tc.keyName, tc.scope, tc.ttl, "owner"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	key, p, err := f.svc.CreateAPIKey(t.Context(), f.order.ID, "k", nil, 24*time.Hour, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if key.KeyHash != domain.HashAPIKey(p) || key.Prefix != p[:12] || key.Source != domain.APIKeySourceAPI ||
		key.ExpiresAt == nil || !key.ExpiresAt.Equal(f.now.Add(24*time.Hour)) || key.CreatedBy != "owner" {
		t.Fatalf("key = %+v", key)
	}
	if p[:8] != "dk_test_" {
		t.Fatalf("minted with the configured env marker: %q", p[:8])
	}
}

func TestSyncEnvAPIKey_Lifecycle(t *testing.T) {
	f := newAPIKeyFixture(t)
	ctx := t.Context()
	k1, _ := domain.GenerateAPIKey(domain.APIKeyEnvLive)
	k2, _ := domain.GenerateAPIKey(domain.APIKeyEnvLive)
	exchanges := func(p string) bool { _, err := f.svc.ExchangeAPIKey(ctx, p); return err == nil }
	sync := func(p, want string) {
		t.Helper()
		got, err := f.svc.SyncEnvAPIKey(ctx, f.order.ID, p)
		if err != nil || got != want {
			t.Fatalf("SyncEnvAPIKey(%.12s) = %q, %v; want %q", p, got, err, want)
		}
	}
	// An operator-minted key is never touched by env syncing.
	_, minted, _ := f.svc.CreateAPIKey(ctx, f.order.ID, "ops", nil, 0, "owner")

	sync(k1, "seeded")
	sync(k1, "") // every later boot
	if !exchanges(k1) {
		t.Fatal("seeded key does not exchange")
	}
	envKey, _ := f.keys.FindByHash(ctx, domain.HashAPIKey(k1))
	if _, err := f.svc.RevokeAPIKey(ctx, envKey.ID); !errors.Is(err, autherrors.ErrEnvManagedKey) {
		t.Fatalf("revoking an env key: %v, want ErrEnvManagedKey", err)
	}

	sync(k2, "rotated")
	if exchanges(k1) || !exchanges(k2) {
		t.Fatal("rotation must retire the old key and enable the new one")
	}
	sync("", "revoked")
	if exchanges(k2) {
		t.Fatal("unsetting the env var must revoke the key")
	}
	sync("", "")
	sync(k2, "seeded") // set back: the same row comes back
	if !exchanges(k2) || !exchanges(minted) {
		t.Fatal("re-enabled env key or the minted key stopped working")
	}
	if _, err := f.svc.SyncEnvAPIKey(ctx, f.order.ID, "hunter2"); err == nil {
		t.Fatal("a malformed env key must fail loudly at boot")
	}
}

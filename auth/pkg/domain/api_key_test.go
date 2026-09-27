package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func TestGenerateAPIKey_FormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		k, err := GenerateAPIKey(APIKeyEnvLive)
		if err != nil {
			t.Fatalf("GenerateAPIKey: %v", err)
		}
		if !strings.HasPrefix(k, "dk_live_") || !ValidAPIKeyFormat(k) {
			t.Fatalf("bad key %q", k)
		}
		if seen[k] {
			t.Fatal("duplicate key")
		}
		seen[k] = true
	}
	test, _ := GenerateAPIKey(APIKeyEnvTest)
	if !strings.HasPrefix(test, "dk_test_") || !ValidAPIKeyFormat(test) {
		t.Fatalf("bad test key %q", test)
	}
	if _, err := GenerateAPIKey("prod"); err == nil {
		t.Fatal("unknown env must be refused")
	}
}

func TestValidAPIKeyFormat(t *testing.T) {
	good, _ := GenerateAPIKey(APIKeyEnvLive)
	for _, bad := range []string{
		"", "password", "dk_live_short", "dk_prod_" + good[8:], good + "x", good[:len(good)-1],
		"dk_live_" + strings.Repeat("!", 43),
	} {
		if ValidAPIKeyFormat(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestHashAPIKey_StableAndPrefixed(t *testing.T) {
	k, _ := GenerateAPIKey(APIKeyEnvLive)
	if HashAPIKey(k) != HashAPIKey(k) || len(HashAPIKey(k)) != 64 {
		t.Fatal("hash must be a stable 64-char hex digest")
	}
	if HashAPIKey(k) == HashAPIKey(k+"x") {
		t.Fatal("different keys hashed alike")
	}
	if p := APIKeyPrefix(k); p != k[:12] || !strings.HasPrefix(p, "dk_live_") {
		t.Fatalf("prefix = %q", p)
	}
}

func TestExchangeRejection(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	svc := &User{ID: "s", AccountType: AccountTypeService, IsActive: true}
	cases := []struct {
		name  string
		key   *APIKey
		owner *User
		want  APIKeyRejection
	}{
		{"ok", &APIKey{}, svc, ""},
		{"ok before expiry", &APIKey{ExpiresAt: &future}, svc, ""},
		{"unknown", nil, nil, APIKeyUnknown},
		{"revoked", &APIKey{RevokedAt: &past}, svc, APIKeyRevoked},
		{"expired", &APIKey{ExpiresAt: &past}, svc, APIKeyExpired},
		{"owner gone", &APIKey{}, nil, APIKeyInactive},
		{"owner inactive", &APIKey{}, &User{AccountType: AccountTypeService}, APIKeyInactive},
		{"owner is a person", &APIKey{}, &User{AccountType: AccountTypeManager, IsActive: true}, APIKeyNotService},
		// Lockout defends passwords; it must not gate keys (see ExchangeRejection).
		{"owner locked", &APIKey{}, &User{AccountType: AccountTypeService, IsActive: true, LockedAt: &now}, ""},
	}
	for _, tc := range cases {
		if got := ExchangeRejection(tc.key, tc.owner, now); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEffectivePermissions(t *testing.T) {
	owner := &User{Permissions: []string{permissions.ProductAll, permissions.OrderShip}}
	if got := EffectivePermissions(&APIKey{}, owner); len(got) != 2 {
		t.Fatalf("empty scope inherits: %v", got)
	}
	got := EffectivePermissions(&APIKey{Permissions: []string{permissions.ProductCreate, permissions.PaymentCancel}}, owner)
	if len(got) != 1 || got[0] != permissions.ProductCreate {
		t.Fatalf("scope ∩ account = %v, want [product.create] (wildcard-aware, payment.cancel dropped)", got)
	}
	// Narrowing the account narrows the key at the next exchange.
	owner.Permissions = []string{permissions.OrderShip}
	if got := EffectivePermissions(&APIKey{Permissions: []string{permissions.ProductCreate}}, owner); len(got) != 0 {
		t.Fatalf("after narrowing: %v", got)
	}
}

func TestScopeWithinAccount(t *testing.T) {
	owner := &User{Permissions: []string{permissions.ProductAll}}
	if p := ScopeWithinAccount([]string{permissions.ProductCreate, permissions.ProductDelete}, owner); p != "" {
		t.Fatalf("covered scope refused at %q", p)
	}
	if p := ScopeWithinAccount([]string{permissions.ProductCreate, permissions.OrderShip}, owner); p != permissions.OrderShip {
		t.Fatalf("got %q, want order.ship", p)
	}
}

func TestRetirePassword(t *testing.T) {
	u, _ := NewUser("u", "svc@example.com", "old-password", AccountTypeService)
	if !u.ValidatePassword("old-password") {
		t.Fatal("setup")
	}
	u.RetirePassword()
	if !u.PasswordRetired() {
		t.Fatal("not retired")
	}
	for _, pw := range []string{"old-password", "", "!"} {
		if u.ValidatePassword(pw) {
			t.Fatalf("%q validated against a retired password", pw)
		}
	}
	_ = u.UpdatePassword("new-password")
	if u.PasswordRetired() || !u.ValidatePassword("new-password") {
		t.Fatal("setting a password must restore password login")
	}
}

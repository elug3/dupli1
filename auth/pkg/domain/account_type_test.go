package domain_test

import (
	"testing"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func TestNewUserSetsAccountType(t *testing.T) {
	u, err := domain.NewUser("id-1", "user@example.com", "supersecret", domain.AccountTypeManager, permissions.UserRead)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if u.AccountType != domain.AccountTypeManager {
		t.Fatalf("AccountType = %q, want %q", u.AccountType, domain.AccountTypeManager)
	}
}

func TestValidAccountType(t *testing.T) {
	for _, tt := range []struct {
		value string
		ok    bool
	}{
		{domain.AccountTypeCustomer, true},
		{domain.AccountTypeManager, true},
		{domain.AccountTypeService, true},
		{"admin", false}, // permission tier, not account_type
		{"", false},
		{"staff", false},
	} {
		if got := domain.ValidAccountType(tt.value); got != tt.ok {
			t.Errorf("ValidAccountType(%q) = %v, want %v", tt.value, got, tt.ok)
		}
	}
}

func TestNormalizeAccountType_LegacyDBAdmin(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{"", ""},
		{domain.AccountTypeCustomer, domain.AccountTypeCustomer},
		{domain.AccountTypeManager, domain.AccountTypeManager},
		{domain.AccountTypeService, domain.AccountTypeService},
		{"admin", domain.AccountTypeManager}, // stale DB row only
		{"staff", "staff"},
	} {
		if got := domain.NormalizeAccountType(tt.in); got != tt.want {
			t.Errorf("NormalizeAccountType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUserClassTreatsStaleAdminAccountTypeAsManagerTier(t *testing.T) {
	// Stale DB row before migrate; ABAC still classifies via NormalizeAccountType.
	u, err := domain.NewUser("id-1", "ops@example.com", "supersecret", "admin", permissions.AdminAll)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if got := domain.UserClass(u); got != domain.ClassAdmin {
		t.Fatalf("UserClass = %v, want ClassAdmin for stale account_type admin + admin.*", got)
	}
}

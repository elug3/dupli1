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
		{domain.AccountTypeAdminLegacy, false}, // must normalize first
		{"", false},
		{"staff", false},
	} {
		if got := domain.ValidAccountType(tt.value); got != tt.ok {
			t.Errorf("ValidAccountType(%q) = %v, want %v", tt.value, got, tt.ok)
		}
	}
}

func TestNormalizeAccountType(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{"", ""},
		{domain.AccountTypeCustomer, domain.AccountTypeCustomer},
		{domain.AccountTypeManager, domain.AccountTypeManager},
		{domain.AccountTypeService, domain.AccountTypeService},
		{domain.AccountTypeAdminLegacy, domain.AccountTypeManager},
		{"staff", "staff"},
	} {
		if got := domain.NormalizeAccountType(tt.in); got != tt.want {
			t.Errorf("NormalizeAccountType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUserClassAcceptsLegacyAdminAccountType(t *testing.T) {
	u, err := domain.NewUser("id-1", "ops@example.com", "supersecret", domain.AccountTypeAdminLegacy, permissions.AdminAll)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if got := domain.UserClass(u); got != domain.ClassAdmin {
		t.Fatalf("UserClass = %v, want ClassAdmin for legacy account_type admin + admin.*", got)
	}
}

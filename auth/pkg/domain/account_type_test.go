package domain_test

import (
	"testing"

	"github.com/elug3/dupli1/auth/pkg/domain"
)

func TestNewUserSetsAccountType(t *testing.T) {
	u, err := domain.NewUser("id-1", "user@example.com", "supersecret", domain.AccountTypeAdmin, domain.RoleAdmin)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if u.AccountType != domain.AccountTypeAdmin {
		t.Fatalf("AccountType = %q, want %q", u.AccountType, domain.AccountTypeAdmin)
	}
}

func TestValidAccountType(t *testing.T) {
	for _, tt := range []struct {
		value string
		ok    bool
	}{
		{domain.AccountTypeCustomer, true},
		{domain.AccountTypeAdmin, true},
		{domain.AccountTypeService, true},
		{"", false},
		{"staff", false},
	} {
		if got := domain.ValidAccountType(tt.value); got != tt.ok {
			t.Errorf("ValidAccountType(%q) = %v, want %v", tt.value, got, tt.ok)
		}
	}
}

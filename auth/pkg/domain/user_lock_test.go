package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func TestLockExemptAdminAndOwner(t *testing.T) {
	owner, _ := domain.NewUser("o1", "owner@dupli1.com", "password12", domain.AccountTypeAdmin, permissions.All)
	admin, _ := domain.NewUser("a1", "admin@dupli1.com", "password12", domain.AccountTypeAdmin, permissions.AdminAll)
	customer, _ := domain.NewUser("c1", "c@example.com", "password12", domain.AccountTypeCustomer)
	manager, _ := domain.NewUser("m1", "m@dupli1.com", "password12", domain.AccountTypeAdmin, permissions.UserRead)

	if !owner.IsLockExempt() || !admin.IsLockExempt() {
		t.Fatal("owner and admin must be lock-exempt")
	}
	if customer.IsLockExempt() || manager.IsLockExempt() {
		t.Fatal("customer and manager must not be lock-exempt")
	}

	now := time.Now()
	owner.LockedAt = &now
	admin.Lock() // no-op
	customer.Lock()

	if owner.IsLocked() || admin.IsLocked() {
		t.Fatal("exempt accounts must never report IsLocked")
	}
	if admin.LockedAt != nil {
		t.Fatal("Lock() must not set LockedAt on admin")
	}
	if !customer.IsLocked() {
		t.Fatal("customer should lock normally")
	}
}

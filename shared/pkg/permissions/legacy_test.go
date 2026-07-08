package permissions

import (
	"slices"
	"testing"
)

func TestExpandLegacyRoles_owner(t *testing.T) {
	got := ExpandLegacyRoles([]string{RoleOwner})
	if len(got) != 1 || got[0] != All {
		t.Fatalf("owner = %v, want [*]", got)
	}
}

func TestExpandLegacyRoles_productManager(t *testing.T) {
	got := ExpandLegacyRoles([]string{RoleProductManager})
	want := []string{ProductAll, CouponAll}
	if !slices.Equal(got, want) {
		t.Fatalf("product_manager = %v, want %v", got, want)
	}
}

func TestExpandLegacyRoles_orderManager(t *testing.T) {
	got := ExpandLegacyRoles([]string{RoleOrderManager})
	for _, p := range []string{OrderShip, OrderStatusUpdate, OrderReadAll, InventoryStockWrite, InventoryReservationManage, CartRead} {
		if !slices.Contains(got, p) {
			t.Fatalf("order_manager missing %s in %v", p, got)
		}
	}
}

func TestExpandLegacyRoles_admin(t *testing.T) {
	got := ExpandLegacyRoles([]string{RoleAdmin})
	for _, p := range []string{AdminAll, ProductAll, CouponAll, OrderShip, CartRead} {
		if !slices.Contains(got, p) {
			t.Fatalf("admin missing %s in %v", p, got)
		}
	}
}

func TestExpandLegacyRoles_union(t *testing.T) {
	got := ExpandLegacyRoles([]string{RoleUserManager, RoleCustomerRegistrar})
	if !slices.Contains(got, UserCreate) {
		t.Fatal("expected user.create from registrar")
	}
	if !slices.Contains(got, UserPasswordUpdate) {
		t.Fatal("expected user.password.update from user_manager")
	}
}

func TestExpandLegacyRoles_customerEmpty(t *testing.T) {
	got := ExpandLegacyRoles([]string{RoleCustomer})
	if len(got) != 0 {
		t.Fatalf("customer = %v, want empty", got)
	}
}

func TestExpandLegacyRoles_unknownIgnored(t *testing.T) {
	got := ExpandLegacyRoles([]string{"unknown_role", RoleCustomerRegistrar})
	if !slices.Equal(got, []string{UserCreate}) {
		t.Fatalf("got %v", got)
	}
}

func TestIsLegacyRole(t *testing.T) {
	if !IsLegacyRole(RoleAdmin) {
		t.Fatal("admin should be legacy")
	}
	if IsLegacyRole("product.create") {
		t.Fatal("permission string is not a legacy role")
	}
}

// Contract: legacy product_manager must grant product.create via wildcard.
func TestLegacyProductManagerGrantsProductCreate(t *testing.T) {
	perms := ExpandLegacyRoles([]string{RoleProductManager})
	if !Has(perms, ProductCreate) {
		t.Fatal("product_manager expansion must grant product.create")
	}
}

// Contract: legacy order_manager must grant order.ship.
func TestLegacyOrderManagerGrantsOrderShip(t *testing.T) {
	perms := ExpandLegacyRoles([]string{RoleOrderManager})
	if !Has(perms, OrderShip) {
		t.Fatal("order_manager expansion must grant order.ship")
	}
}

func TestNeedsExpansion(t *testing.T) {
	if !NeedsExpansion([]string{RoleAdmin}) {
		t.Fatal("admin role should need expansion")
	}
	if NeedsExpansion([]string{ProductCreate}) {
		t.Fatal("concrete permission should not need expansion")
	}
}

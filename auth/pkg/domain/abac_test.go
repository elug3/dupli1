package domain

import (
	"testing"

	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func TestCanManage_hierarchy(t *testing.T) {
	cases := []struct {
		caller, target ManagementClass
		want           bool
	}{
		{ClassOwner, ClassAdmin, true},
		{ClassOwner, ClassManager, true},
		{ClassOwner, ClassCustomer, true},
		{ClassOwner, ClassOwner, false},
		{ClassOwner, ClassService, true},
		{ClassAdmin, ClassService, false},
		{ClassManager, ClassService, false},
		{ClassAdmin, ClassAdmin, false},
		{ClassAdmin, ClassManager, true},
		{ClassAdmin, ClassCustomer, true},
		{ClassManager, ClassManager, false},
		{ClassManager, ClassCustomer, true},
		{ClassCustomer, ClassCustomer, false},
	}
	for _, tc := range cases {
		if got := CanManage(tc.caller, tc.target); got != tc.want {
			t.Fatalf("CanManage(%v, %v) = %v, want %v", tc.caller, tc.target, got, tc.want)
		}
	}
}

func TestCallerClass_fromPermissions(t *testing.T) {
	if CallerClass([]string{permissions.All}) != ClassOwner {
		t.Fatal("expected owner")
	}
	adminPerms := permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})
	if CallerClass(adminPerms) != ClassAdmin {
		t.Fatalf("admin perms class = %v", CallerClass(adminPerms))
	}
	managerPerms := permissions.ExpandLegacyRoles([]string{permissions.RoleUserManager})
	if CallerClass(managerPerms) != ClassManager {
		t.Fatalf("manager perms class = %v", CallerClass(managerPerms))
	}
}

func TestUserClass_customerAndManager(t *testing.T) {
	customer, _ := NewUser("c1", "c@example.com", "password12", AccountTypeCustomer)
	if UserClass(customer) != ClassCustomer {
		t.Fatalf("customer class = %v", UserClass(customer))
	}

	manager, _ := NewUser("m1", "m@example.com", "password12", AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate)
	if UserClass(manager) != ClassManager {
		t.Fatalf("manager class = %v", UserClass(manager))
	}

	admin, _ := NewUser("a1", "a@example.com", "password12", AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	if UserClass(admin) != ClassAdmin {
		t.Fatalf("admin class = %v", UserClass(admin))
	}
}

func TestCanRegister_registrarOnlyCustomer(t *testing.T) {
	registrar, _ := NewUser("r1", "r@internal", "password12", AccountTypeService, permissions.UserCreate)
	if !CanRegister(registrar, AccountTypeCustomer, nil) {
		t.Fatal("registrar should register customer")
	}
	if CanRegister(registrar, AccountTypeManager, nil) {
		t.Fatal("registrar should not register manager account type")
	}
	if CanRegister(registrar, "admin", nil) {
		t.Fatal("registrar should not register admin (not an account_type; treated as manager tier)")
	}
}

func TestCanAssignPermissions_adminCannotPromoteToOwner(t *testing.T) {
	admin, _ := NewUser("a1", "a@example.com", "password12", AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	manager, _ := NewUser("m1", "m@example.com", "password12", AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate)

	if !CanAssignPermissions(admin, manager, []string{permissions.UserPasswordUpdate, permissions.UserStatusUpdate}, "") {
		t.Fatal("admin should assign manager permissions")
	}
	if CanAssignPermissions(admin, manager, []string{permissions.All}, "") {
		t.Fatal("admin must not assign owner permissions")
	}
}

func TestCanAssignPermissions_adminCannotPromoteToAdmin(t *testing.T) {
	admin, _ := NewUser("a1", "a@example.com", "password12", AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	manager, _ := NewUser("m1", "m@example.com", "password12", AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate)

	adminPerms := permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})
	if CanAssignPermissions(admin, manager, adminPerms, "") {
		t.Fatal("admin must not promote manager to admin tier")
	}
}

func TestCanManageUser_blocksSelf(t *testing.T) {
	u, _ := NewUser("u1", "u@example.com", "password12", AccountTypeManager, permissions.All)
	if CanManageUser(u, u) {
		t.Fatal("user must not manage self")
	}
}

func TestUserClass_serviceAccountIsOwnerManaged(t *testing.T) {
	svc, _ := NewUser("s1", "order@internal", "password12", AccountTypeService,
		permissions.OrderStatusUpdate, permissions.PaymentCancel, permissions.PromotionRedeem)
	if UserClass(svc) != ClassService {
		t.Fatalf("service class = %v", UserClass(svc))
	}
	owner, _ := NewUser("o1", "o@example.com", "password12", AccountTypeManager, permissions.All)
	admin, _ := NewUser("a1", "a@example.com", "password12", AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	manager, _ := NewUser("m1", "m@example.com", "password12", AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate)

	if !CanManageUser(owner, svc) {
		t.Fatal("owner should manage a service account")
	}
	if CanManageUser(admin, svc) {
		t.Fatal("admin must not manage a service account")
	}
	if CanManageUser(manager, svc) {
		t.Fatal("manager must not reset a service account's password")
	}
	if CanRegister(admin, AccountTypeService, nil) {
		t.Fatal("admin must not register a service account")
	}
	customer, _ := NewUser("c1", "c@example.com", "password12", AccountTypeCustomer)
	if CanAssignPermissions(admin, customer, []string{permissions.ProductRead}, AccountTypeService) {
		t.Fatal("admin must not convert an account to service")
	}
}

func TestUserClass_permissionsRaiseCustomerTier(t *testing.T) {
	c, _ := NewUser("c1", "c@example.com", "password12", AccountTypeCustomer, permissions.AdminAll)
	if UserClass(c) != ClassAdmin {
		t.Fatalf("customer-typed admin.* class = %v, want admin", UserClass(c))
	}
	if c.IsLockExempt() {
		t.Fatal("lockout exemption stays with manager-typed admins")
	}
}

func TestCanAssignPermissions_managerCannotMintAdmin(t *testing.T) {
	manager, _ := NewUser("m1", "m@example.com", "password12", AccountTypeManager,
		permissions.UserPasswordUpdate, permissions.UserStatusUpdate, permissions.UserPermissionsUpdate)
	customer, _ := NewUser("c1", "c@example.com", "password12", AccountTypeCustomer)

	if CanAssignPermissions(manager, customer, []string{permissions.AdminAll}, "") {
		t.Fatal("manager must not grant admin.* to a customer-typed account")
	}
	if CanAssignPermissions(manager, customer, []string{permissions.PaymentCancel}, "") {
		t.Fatal("manager must not grant a permission it does not hold")
	}
	if !CanAssignPermissions(manager, customer, []string{permissions.UserStatusUpdate}, "") {
		t.Fatal("manager should grant a customer-tier permission it holds")
	}
}

func TestCanAssignPermissions_wildcardHolderGrantsCovered(t *testing.T) {
	admin, _ := NewUser("a1", "a@example.com", "password12", AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	manager, _ := NewUser("m1", "m@example.com", "password12", AccountTypeManager)

	if !CanAssignPermissions(admin, manager, []string{permissions.ProductCreate, permissions.PromotionAll}, "") {
		t.Fatal("admin holding product.* and promotion.* should grant within them")
	}
	if CanAssignPermissions(admin, manager, []string{permissions.SupportRead}, "") {
		t.Fatal("admin must not grant support.read without holding it")
	}
}

func TestCanRegister_requiresHeldPermissions(t *testing.T) {
	admin, _ := NewUser("a1", "a@example.com", "password12", AccountTypeManager,
		permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})...)
	if CanRegister(admin, AccountTypeManager, []string{permissions.PaymentCancel}) {
		t.Fatal("admin must not register an account with a permission it does not hold")
	}
	if !CanRegister(admin, AccountTypeManager, []string{permissions.OrderShip}) {
		t.Fatal("admin should register a manager with a permission it holds")
	}
}

package permissions

// Legacy RBAC role names replaced by fine-grained permissions.
const (
	RoleOwner             = "owner"
	RoleAdmin             = "admin"
	RoleUserManager       = "user_manager"
	RoleCustomerRegistrar = "customer_registrar"
	RoleProductManager    = "product_manager"
	RoleOrderManager      = "order_manager"
	RoleCustomer          = "customer"
)

var legacyRolePermissions = map[string][]string{
	RoleOwner:             {All},
	RoleAdmin:             adminPermissions(),
	RoleUserManager:       {UserPasswordUpdate, UserStatusUpdate},
	RoleCustomerRegistrar: {UserCreate},
	RoleProductManager:    {ProductAll, CouponAll},
	RoleOrderManager:      orderManagerPermissions(),
	RoleCustomer:          nil,
}

func adminPermissions() []string {
	return []string{
		AdminAll,
		UserCreate,
		UserRead,
		UserPermissionsUpdate,
		UserPasswordUpdate,
		UserStatusUpdate,
		ProductAll,
		CouponAll,
		InventoryStockWrite,
		InventoryReservationManage,
		OrderShip,
		OrderStatusUpdate,
		OrderReadAll,
		CartRead,
	}
}

func orderManagerPermissions() []string {
	return []string{
		OrderShip,
		OrderStatusUpdate,
		OrderReadAll,
		InventoryStockWrite,
		InventoryReservationManage,
		CartRead,
	}
}

// ExpandLegacyRoles maps deprecated role names to permission sets.
// Multiple roles are unioned and deduplicated. Unknown roles are ignored.
func ExpandLegacyRoles(roles []string) []string {
	var out []string
	for _, role := range roles {
		perms, ok := legacyRolePermissions[role]
		if !ok {
			continue
		}
		out = append(out, perms...)
	}
	return Dedupe(out)
}

// IsLegacyRole reports whether name is a known pre-migration role.
func IsLegacyRole(name string) bool {
	_, ok := legacyRolePermissions[name]
	return ok
}

// Resolve returns the effective permission set from JWT claims during dual-read.
// When permissions is non-empty it is returned as-is (deduplicated).
// Otherwise legacy roles are expanded.
func Resolve(permissions, legacyRoles []string) []string {
	if len(permissions) > 0 {
		return Dedupe(permissions)
	}
	return ExpandLegacyRoles(legacyRoles)
}

// InferLegacyRoles maps a permission set to legacy role names for JWT dual-read.
// Downstream services that still read the roles claim use this during migration.
func InferLegacyRoles(perms []string) []string {
	if len(perms) == 0 {
		return []string{RoleCustomer}
	}

	var roles []string
	if Has(perms, All) {
		roles = append(roles, RoleOwner)
	}
	if infersAdmin(perms) {
		roles = append(roles, RoleAdmin)
	}
	if Has(perms, ProductAll) || Has(perms, CouponAll) {
		roles = append(roles, RoleProductManager)
	}
	if infersOrderManager(perms) {
		roles = append(roles, RoleOrderManager)
	}
	if infersUserManager(perms) {
		roles = append(roles, RoleUserManager)
	}
	if infersCustomerRegistrar(perms) {
		roles = append(roles, RoleCustomerRegistrar)
	}
	return Dedupe(roles)
}

func infersAdmin(perms []string) bool {
	return Has(perms, AdminAll) ||
		(Has(perms, UserRead) && Has(perms, UserPermissionsUpdate) && Has(perms, ProductAll))
}

func infersOrderManager(perms []string) bool {
	return HasAny(perms, OrderShip, OrderStatusUpdate) &&
		Has(perms, InventoryReservationManage)
}

func infersUserManager(perms []string) bool {
	return Has(perms, UserPasswordUpdate) && Has(perms, UserStatusUpdate) && !Has(perms, UserRead)
}

func infersCustomerRegistrar(perms []string) bool {
	return Has(perms, UserCreate) &&
		!Has(perms, UserRead) &&
		!Has(perms, UserPermissionsUpdate) &&
		!Has(perms, AdminAll) &&
		!Has(perms, All)
}

// NeedsExpansion reports whether stored values are legacy role names
// that should be expanded to fine-grained permissions.
func NeedsExpansion(stored []string) bool {
	for _, entry := range stored {
		if IsLegacyRole(entry) {
			return true
		}
	}
	return false
}

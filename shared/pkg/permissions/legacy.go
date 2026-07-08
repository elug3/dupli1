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

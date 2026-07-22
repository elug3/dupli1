package domain

// Account type labels stored on User.AccountType.
// Canonical values: customer | manager | service.
// "admin" is not an account type — it is a permission/management tier (admin.*, ClassAdmin).
const (
	AccountTypeCustomer = "customer"
	AccountTypeManager  = "manager"
	AccountTypeService  = "service"

	// AccountTypeAdminLegacy is accepted on write APIs during a short compat window.
	// NormalizeAccountType maps it to AccountTypeManager before validation/persist.
	// manage-web must stop sending manager→admin on the wire; remove this alias afterward.
	AccountTypeAdminLegacy = "admin"
)

// AllAccountTypes lists supported account_type values in API order (canonical only).
var AllAccountTypes = []string{
	AccountTypeCustomer,
	AccountTypeManager,
	AccountTypeService,
}

// NormalizeAccountType maps legacy wire values to canonical account types.
// Empty string is left unchanged (callers apply DefaultAccountType separately).
func NormalizeAccountType(t string) string {
	if t == AccountTypeAdminLegacy {
		return AccountTypeManager
	}
	return t
}

// ValidAccountType reports whether t is a supported canonical account type.
func ValidAccountType(t string) bool {
	switch t {
	case AccountTypeCustomer, AccountTypeManager, AccountTypeService:
		return true
	default:
		return false
	}
}

// DefaultAccountType is used when register omits account_type.
const DefaultAccountType = AccountTypeCustomer

package domain

// Account type labels stored on User.AccountType.
// Canonical values: customer | manager | service.
//
// "admin" is NOT an account type. It is a permission / management tier
// (permission string admin.*, ABAC ClassAdmin). Clients must send manager
// for human operators. Startup migrate rewrites any leftover DB rows from
// account_type=admin → manager.
const (
	AccountTypeCustomer = "customer"
	AccountTypeManager  = "manager"
	AccountTypeService  = "service"
)

// AllAccountTypes lists supported account_type values in API order.
var AllAccountTypes = []string{
	AccountTypeCustomer,
	AccountTypeManager,
	AccountTypeService,
}

// legacyAccountTypeAdmin is only recognized when reading stale DB rows
// (pre-migrate). Write APIs must not accept it — use ValidAccountType.
const legacyAccountTypeAdmin = "admin"

// NormalizeAccountType maps legacy stored values to canonical account types
// for ABAC classification. Empty string is left unchanged.
// Do not use this to accept "admin" on write APIs.
func NormalizeAccountType(t string) string {
	if t == legacyAccountTypeAdmin {
		return AccountTypeManager
	}
	return t
}

// ValidAccountType reports whether t is a supported canonical account type.
// "admin" is invalid — use manager.
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

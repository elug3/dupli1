package domain

// Account type labels stored on User.AccountType.
const (
	AccountTypeCustomer = "customer"
	AccountTypeAdmin    = "admin"
	AccountTypeService  = "service"
)

// AllAccountTypes lists supported account_type values in API order.
var AllAccountTypes = []string{
	AccountTypeCustomer,
	AccountTypeAdmin,
	AccountTypeService,
}

// ValidAccountType reports whether t is a supported account type.
func ValidAccountType(t string) bool {
	switch t {
	case AccountTypeCustomer, AccountTypeAdmin, AccountTypeService:
		return true
	default:
		return false
	}
}

// DefaultAccountType is used when register omits account_type.
const DefaultAccountType = AccountTypeCustomer

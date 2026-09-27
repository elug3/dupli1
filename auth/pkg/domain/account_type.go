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

// Login clients name the front end a login is for. Auth decides whether an
// account type may sign in there, so the rule holds whatever a web app does;
// the app only shows the message it gets back.
const (
	// ClientStorefront is dupli1-web: customers, and managers shopping.
	ClientStorefront = "storefront"
	// ClientManage is manage-web: managers only.
	ClientManage = "manage"
	// ClientService is a machine login (no web session): service accounts only.
	ClientService = "service"
)

// ValidClient reports whether c is a known login client. Empty is accepted
// while callers roll over to sending one; it applies no account-type rule.
func ValidClient(c string) bool {
	switch c {
	case "", ClientStorefront, ClientManage, ClientService:
		return true
	default:
		return false
	}
}

// ClientRejection returns why accountType may not sign in through client, as
// a message fit to show the person signing in, or "" when it may.
func ClientRejection(client, accountType string) string {
	at := NormalizeAccountType(accountType)
	switch client {
	case ClientStorefront:
		if at == AccountTypeService {
			return "Service accounts cannot sign in to the storefront."
		}
	case ClientManage:
		switch at {
		case AccountTypeManager:
		case AccountTypeService:
			return "Service accounts cannot sign in to manage-web."
		default:
			return "Customer accounts cannot sign in to manage-web. Please sign in on the storefront."
		}
	case ClientService:
		if at != AccountTypeService {
			return "Only service accounts can sign in as a service."
		}
	}
	return ""
}

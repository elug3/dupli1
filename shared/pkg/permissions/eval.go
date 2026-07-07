package permissions

import "strings"

// Has reports whether held grants the required permission.
//
// Evaluation order (per docs/permissions.md):
//  1. exact match
//  2. resource wildcard (e.g. product.*)
//  3. admin.* (grants user.* domain)
//  4. * (grants everything)
func Has(held []string, required string) bool {
	return HasAny(held, required)
}

// HasAny reports whether held grants at least one of the required permissions.
func HasAny(held []string, required ...string) bool {
	if len(required) == 0 {
		return true
	}
	for _, want := range required {
		if grants(held, want) {
			return true
		}
	}
	return false
}

// HasAll reports whether held grants every required permission.
func HasAll(held []string, required ...string) bool {
	for _, want := range required {
		if !grants(held, want) {
			return false
		}
	}
	return true
}

func grants(held []string, required string) bool {
	for _, h := range held {
		if h == required {
			return true
		}
	}
	for _, h := range held {
		if resourceWildcardGrants(h, required) {
			return true
		}
	}
	for _, h := range held {
		if h == AdminAll && strings.HasPrefix(required, "user.") {
			return true
		}
	}
	for _, h := range held {
		if h == All {
			return true
		}
	}
	return false
}

// resourceWildcardGrants reports whether wildcard token h (e.g. "product.*")
// grants the concrete permission required.
func resourceWildcardGrants(wildcard, required string) bool {
	if wildcard == All || wildcard == AdminAll {
		return false
	}
	if !strings.HasSuffix(wildcard, ".*") {
		return false
	}
	prefix := strings.TrimSuffix(wildcard, ".*")
	if prefix == "" {
		return false
	}
	return required == prefix || strings.HasPrefix(required, prefix+".")
}

// CanRegisterAnyAccountType reports whether held can set any account_type at register.
// Callers with only user.create (no admin.* or *) may register customers only.
func CanRegisterAnyAccountType(held []string) bool {
	return HasAny(held, All, AdminAll, UserPermissionsUpdate)
}

// BypassesOrderABAC reports whether held may access orders for any customer_id.
func BypassesOrderABAC(held []string) bool {
	return HasAny(held, All, AdminAll, OrderCreate, OrderReadAll)
}

// BypassesPaymentABAC reports whether held may access payments for any user.
func BypassesPaymentABAC(held []string) bool {
	return HasAny(held, All, AdminAll, PaymentCreate, PaymentReadAll)
}

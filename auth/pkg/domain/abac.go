package domain

import (
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// ManagementClass is the auth-service user hierarchy tier used for ABAC.
// Only dupli1-auth applies these rules; other services use fine-grained permissions.
type ManagementClass int

const (
	ClassCustomer ManagementClass = iota
	ClassManager
	ClassAdmin
	// ClassService is a machine account. Only the owner manages one: it holds
	// cross-service permissions (refunds, ledger moves) that no operator tier
	// is scoped to, so resetting its password would hand those over.
	ClassService
	ClassOwner
)

// CallerClass derives the caller's management tier from stored permissions.
func CallerClass(perms []string) ManagementClass {
	if permissions.Has(perms, permissions.All) {
		return ClassOwner
	}
	if isAdminLevel(perms) {
		return ClassAdmin
	}
	if isManagerLevel(perms) {
		return ClassManager
	}
	return ClassCustomer
}

// UserClass classifies an existing user for management ABAC.
func UserClass(u *User) ManagementClass {
	if u == nil {
		return ClassCustomer
	}
	return classify(u.AccountType, u.Permissions)
}

// ClassFromNewUser classifies a user that would be created with accountType and permissions.
func ClassFromNewUser(accountType string, perms []string) ManagementClass {
	if accountType == "" {
		accountType = DefaultAccountType
	}
	return classify(accountType, perms)
}

// ClassFromPermissions classifies a user after a permission assignment.
func ClassFromPermissions(accountType string, perms []string) ManagementClass {
	return ClassFromNewUser(accountType, perms)
}

// classify is the higher of the tier the account type implies and the tier
// the permissions confer, so a customer-typed account holding admin.* is
// classified admin — account_type alone never lowers a tier.
func classify(accountType string, perms []string) ManagementClass {
	if permissions.Has(perms, permissions.All) {
		return ClassOwner
	}
	base := ClassCustomer
	switch NormalizeAccountType(accountType) {
	case AccountTypeService:
		return ClassService
	case AccountTypeManager:
		base = ClassManager
	}
	if held := CallerClass(perms); held > base {
		return held
	}
	return base
}

// CanManage reports whether callerClass may administer targetClass.
//
// Rules:
//   - owner manages admin, service, manager, and customer (not other owners)
//   - admin manages manager and customer
//   - manager manages customer only
func CanManage(callerClass, targetClass ManagementClass) bool {
	switch callerClass {
	case ClassOwner:
		return targetClass != ClassOwner
	case ClassAdmin:
		return targetClass == ClassManager || targetClass == ClassCustomer
	case ClassManager:
		return targetClass == ClassCustomer
	default:
		return false
	}
}

// IsRegistrarOnly reports a machine/service account that may register customers only.
func IsRegistrarOnly(perms []string) bool {
	return permissions.Has(perms, permissions.UserCreate) &&
		!permissions.Has(perms, permissions.UserPasswordUpdate) &&
		!permissions.Has(perms, permissions.UserStatusUpdate) &&
		!isAdminLevel(perms) &&
		!permissions.Has(perms, permissions.All)
}

// CanRegister reports whether caller may create a user with the given account type and permissions.
// The caller must itself hold every permission in newPerms.
func CanRegister(caller *User, accountType string, newPerms []string) bool {
	if caller == nil {
		return false
	}
	if accountType == "" {
		accountType = DefaultAccountType
	}
	accountType = NormalizeAccountType(accountType)
	if !permissions.HasAll(caller.Permissions, newPerms...) {
		return false
	}
	if IsRegistrarOnly(caller.Permissions) {
		return accountType == AccountTypeCustomer && !wouldBeOwner(newPerms)
	}
	if !permissions.Has(caller.Permissions, permissions.UserCreate) {
		return false
	}
	intended := ClassFromNewUser(accountType, newPerms)
	return CanManage(CallerClass(caller.Permissions), intended)
}

// CanManageUser reports whether caller may administer the target user.
func CanManageUser(caller, target *User) bool {
	if caller == nil || target == nil {
		return false
	}
	if caller.ID == target.ID {
		return false
	}
	return CanManage(CallerClass(caller.Permissions), UserClass(target))
}

// CanAssignPermissions reports whether caller may set newPerms on target with optional accountType change.
// The caller must itself hold every permission in newPerms, and the resulting
// tier (see classify) must be one the caller may manage.
func CanAssignPermissions(caller, target *User, newPerms []string, accountType string) bool {
	if caller == nil || target == nil {
		return false
	}
	if caller.ID == target.ID {
		return false
	}
	if !CanManageUser(caller, target) {
		return false
	}
	// A caller grants only what it holds itself; otherwise a manager with
	// user.permissions.update could mint permissions it was never given.
	if !permissions.HasAll(caller.Permissions, newPerms...) {
		return false
	}
	at := target.AccountType
	if accountType != "" {
		at = NormalizeAccountType(accountType)
	}
	intended := ClassFromPermissions(at, newPerms)
	return CanManage(CallerClass(caller.Permissions), intended)
}

func isAdminLevel(perms []string) bool {
	return permissions.Has(perms, permissions.AdminAll) ||
		(permissions.Has(perms, permissions.UserRead) && permissions.Has(perms, permissions.UserPermissionsUpdate))
}

func isManagerLevel(perms []string) bool {
	return permissions.Has(perms, permissions.UserPasswordUpdate) &&
		permissions.Has(perms, permissions.UserStatusUpdate)
}

func wouldBeOwner(perms []string) bool {
	return permissions.Has(perms, permissions.All)
}

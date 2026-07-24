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
	if permissions.Has(u.Permissions, permissions.All) {
		return ClassOwner
	}
	switch NormalizeAccountType(u.AccountType) {
	case AccountTypeCustomer:
		return ClassCustomer
	case AccountTypeService:
		return ClassCustomer
	case AccountTypeManager:
		if isAdminLevel(u.Permissions) {
			return ClassAdmin
		}
		return ClassManager
	default:
		return ClassCustomer
	}
}

// ClassFromNewUser classifies a user that would be created with accountType and permissions.
func ClassFromNewUser(accountType string, perms []string) ManagementClass {
	if permissions.Has(perms, permissions.All) {
		return ClassOwner
	}
	if accountType == "" {
		accountType = DefaultAccountType
	}
	switch NormalizeAccountType(accountType) {
	case AccountTypeCustomer, AccountTypeService:
		return ClassCustomer
	case AccountTypeManager:
		if isAdminLevel(perms) {
			return ClassAdmin
		}
		return ClassManager
	default:
		return ClassCustomer
	}
}

// ClassFromPermissions classifies a user after a permission assignment.
func ClassFromPermissions(accountType string, perms []string) ManagementClass {
	return ClassFromNewUser(accountType, perms)
}

// CanManage reports whether callerClass may administer targetClass.
//
// Rules:
//   - owner manages admin, manager, and customer (not other owners)
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
func CanRegister(caller *User, accountType string, newPerms []string) bool {
	if caller == nil {
		return false
	}
	if accountType == "" {
		accountType = DefaultAccountType
	}
	accountType = NormalizeAccountType(accountType)
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

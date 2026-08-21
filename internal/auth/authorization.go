package auth

import (
	"fmt"
	"sort"
	"strings"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type Permission string

const (
	PermissionVoyageDeclare    Permission = "voyage.declare"
	PermissionVoyageRead       Permission = "voyage.read"
	PermissionInspectionManage Permission = "inspection.manage"
	PermissionPassageReserve   Permission = "passage.reserve"
	PermissionPassageOperate   Permission = "passage.operate"
	PermissionAuditRead        Permission = "audit.read"
	PermissionUserManage       Permission = "user.manage"
)

var rolePermissions = map[Role]map[Permission]struct{}{
	RoleCarrier: {
		PermissionVoyageDeclare: {},
		PermissionVoyageRead:    {},
	},
	RoleDispatcher: {
		PermissionVoyageDeclare:  {},
		PermissionVoyageRead:     {},
		PermissionPassageReserve: {},
	},
	RoleCustoms: {
		PermissionVoyageRead:       {},
		PermissionInspectionManage: {},
	},
	RoleLock: {
		PermissionVoyageRead:     {},
		PermissionPassageOperate: {},
	},
	RoleAuditor: {
		PermissionVoyageRead: {},
		PermissionAuditRead:  {},
	},
}

func Authorize(user User, tenantID string, permission Permission) error {
	if user.Disabled || user.ID == "" || user.TenantID == "" {
		return domain.ErrForbidden
	}
	if strings.TrimSpace(tenantID) == "" || user.TenantID != tenantID {
		return fmt.Errorf("%w: tenant boundary", domain.ErrForbidden)
	}
	permissions, ok := rolePermissions[user.Role]
	if !ok {
		return fmt.Errorf("%w: unknown role", domain.ErrForbidden)
	}
	if _, ok := permissions[permission]; !ok {
		return fmt.Errorf("%w: permission %s", domain.ErrForbidden, permission)
	}
	return nil
}

func Permissions(role Role) []Permission {
	permissions := rolePermissions[role]
	result := make([]Permission, 0, len(permissions))
	for permission := range permissions {
		result = append(result, permission)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func ParseRole(value string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(value)))
	if !validRole(role) {
		return "", fmt.Errorf("%w: role", domain.ErrInvalid)
	}
	return role, nil
}

func CanAssume(actor Role, requested Role) bool {
	if actor == requested {
		return true
	}
	if actor != RoleAuditor {
		return false
	}
	return requested == RoleCarrier || requested == RoleDispatcher || requested == RoleCustoms || requested == RoleLock
}

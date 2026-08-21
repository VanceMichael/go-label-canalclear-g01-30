package auth

import (
	"errors"
	"reflect"
	"testing"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func TestRolePermissionsMatchOperationalBoundaries(t *testing.T) {
	tests := []struct {
		role       Role
		permission Permission
		allowed    bool
	}{
		{role: RoleCarrier, permission: PermissionVoyageDeclare, allowed: true},
		{role: RoleCarrier, permission: PermissionInspectionManage, allowed: false},
		{role: RoleDispatcher, permission: PermissionPassageReserve, allowed: true},
		{role: RoleDispatcher, permission: PermissionPassageOperate, allowed: false},
		{role: RoleCustoms, permission: PermissionInspectionManage, allowed: true},
		{role: RoleCustoms, permission: PermissionPassageReserve, allowed: false},
		{role: RoleLock, permission: PermissionPassageOperate, allowed: true},
		{role: RoleAuditor, permission: PermissionAuditRead, allowed: true},
		{role: RoleAuditor, permission: PermissionVoyageDeclare, allowed: false},
	}
	for _, test := range tests {
		t.Run(string(test.role)+"/"+string(test.permission), func(t *testing.T) {
			user := User{ID: "user", TenantID: "tenant-a", Role: test.role}
			err := Authorize(user, "tenant-a", test.permission)
			if test.allowed && err != nil {
				t.Fatalf("unexpected error=%v", err)
			}
			if !test.allowed && !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestAuthorizeEnforcesTenantAndAccountState(t *testing.T) {
	user := User{ID: "user", TenantID: "tenant-a", Role: RoleCarrier}
	if err := Authorize(user, "tenant-b", PermissionVoyageRead); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tenant error=%v", err)
	}
	user.Disabled = true
	if err := Authorize(user, "tenant-a", PermissionVoyageRead); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("disabled error=%v", err)
	}
	user.Disabled = false
	user.ID = ""
	if err := Authorize(user, "tenant-a", PermissionVoyageRead); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("empty id error=%v", err)
	}
}

func TestPermissionsAreSortedAndIsolated(t *testing.T) {
	want := []Permission{PermissionVoyageDeclare, PermissionVoyageRead}
	got := Permissions(RoleCarrier)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("permissions=%v", got)
	}
	got[0] = PermissionUserManage
	if reflect.DeepEqual(Permissions(RoleCarrier), got) {
		t.Fatal("permissions should return an isolated slice")
	}
	if permissions := Permissions(Role("unknown")); len(permissions) != 0 {
		t.Fatalf("unknown permissions=%v", permissions)
	}
}

func TestParseRoleAndAssumptionRules(t *testing.T) {
	role, err := ParseRole(" CUSTOMS_OFFICER ")
	if err != nil || role != RoleCustoms {
		t.Fatalf("role=%q err=%v", role, err)
	}
	if _, err := ParseRole("administrator"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown role error=%v", err)
	}
	if !CanAssume(RoleAuditor, RoleCustoms) || !CanAssume(RoleCarrier, RoleCarrier) {
		t.Fatal("expected permitted role assumptions")
	}
	if CanAssume(RoleCarrier, RoleCustoms) || CanAssume(RoleAuditor, RoleAuditor) == false {
		t.Fatal("unexpected assumption decision")
	}
}

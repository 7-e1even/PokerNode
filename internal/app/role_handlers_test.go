package app

import (
	"errors"
	"net/http"
	"testing"

	"pokernode/internal/access"
)

func TestValidateRoleInputAddsPermissionDependencies(t *testing.T) {
	role, err := validateRoleInput(roleInput{
		Key:         "user_admin",
		Name:        "用户管理员",
		Permissions: []string{string(access.PermissionUsersManage)},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		string(access.PermissionAdminView),
		string(access.PermissionUsersRead),
		string(access.PermissionUsersManage),
	}
	if len(role.Permissions) != len(want) {
		t.Fatalf("permissions = %v, want %v", role.Permissions, want)
	}
	for index := range want {
		if role.Permissions[index] != want[index] {
			t.Fatalf("permissions = %v, want %v", role.Permissions, want)
		}
	}
}

func TestRoleAdministratorCannotGrantPermissionsTheyDoNotHave(t *testing.T) {
	err := validateGrantablePermissions("role_admin", []access.Permission{
		access.PermissionAdminView,
		access.PermissionRolesManage,
	}, []string{
		string(access.PermissionAdminView),
		string(access.PermissionRolesManage),
		string(access.PermissionRankingsManage),
	})
	var apiErr *apiError
	if err == nil || !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
		t.Fatalf("expected forbidden grant ceiling error, got %v", err)
	}
}

func TestSuperAdminCanGrantEveryCatalogPermission(t *testing.T) {
	requested := make([]string, 0, len(access.AllPermissions()))
	for _, permission := range access.AllPermissions() {
		requested = append(requested, string(permission))
	}
	if err := validateGrantablePermissions(string(access.RoleSuperAdmin), nil, requested); err != nil {
		t.Fatal(err)
	}
}

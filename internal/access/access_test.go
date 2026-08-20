package access

import "testing"

func TestBalanceManagementPermission(t *testing.T) {
	if !Has(string(RoleSuperAdmin), PermissionBalancesManage) || !Has(string(RoleOperator), PermissionBalancesManage) {
		t.Fatal("platform administrators should manage balances across channels")
	}
	if Has(string(RolePlayer), PermissionBalancesManage) {
		t.Fatal("players must not receive global balance management permission")
	}
}

func TestExpandedPermissionsIncludeDependencies(t *testing.T) {
	permissions := ExpandPermissions([]Permission{PermissionUsersManage, PermissionRankingsManage})
	granted := make(map[Permission]bool, len(permissions))
	for _, permission := range permissions {
		granted[permission] = true
	}
	for _, expected := range []Permission{PermissionAdminView, PermissionUsersRead, PermissionUsersManage, PermissionRankingsManage} {
		if !granted[expected] {
			t.Fatalf("expanded permissions are missing %q: %v", expected, permissions)
		}
	}
}

func TestCatalogCoversDelegatedAdminFeatures(t *testing.T) {
	for _, permission := range []Permission{PermissionRankingsManage, PermissionAuthSettingsManage, PermissionBrandingManage} {
		if !ValidPermission(string(permission)) {
			t.Fatalf("permission %q is missing from the catalog", permission)
		}
	}
}

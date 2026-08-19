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

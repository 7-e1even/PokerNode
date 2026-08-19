package access

type Role string

const (
	RoleSuperAdmin     Role = "super_admin"
	RoleOperator       Role = "operator"
	RoleChannelManager Role = "channel_manager"
	RolePlayer         Role = "player"
)

type Permission string

const (
	PermissionAdminView          Permission = "admin:view"
	PermissionUsersRead          Permission = "users:read"
	PermissionUsersManage        Permission = "users:manage"
	PermissionChannelsManage     Permission = "channels:manage"
	PermissionBalancesManage     Permission = "balances:manage"
	PermissionRolesManage        Permission = "roles:manage"
	PermissionRegistrationManage Permission = "registration:manage"
)

type PermissionDefinition struct {
	Key         Permission `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Group       string     `json:"group"`
}

var permissionCatalog = []PermissionDefinition{
	{Key: PermissionAdminView, Name: "访问运营后台", Description: "登录并查看运营后台", Group: "后台访问"},
	{Key: PermissionUsersRead, Name: "查看用户", Description: "查看平台用户与账号状态", Group: "用户管理"},
	{Key: PermissionUsersManage, Name: "管理用户", Description: "创建、编辑、停用和删除允许管理的账号", Group: "用户管理"},
	{Key: PermissionChannelsManage, Name: "管理频道", Description: "管理分配频道的连接、牌桌和邀请码", Group: "频道管理"},
	{Key: PermissionBalancesManage, Name: "管理余额", Description: "调整分配频道内成员的 New API 余额", Group: "频道管理"},
	{Key: PermissionRolesManage, Name: "管理角色", Description: "创建角色、配置权限并为用户分配角色", Group: "权限管理"},
	{Key: PermissionRegistrationManage, Name: "管理注册策略", Description: "开启或关闭平台注册入口", Group: "系统设置"},
}

var rolePermissions = map[Role]map[Permission]struct{}{
	RoleSuperAdmin: {
		PermissionAdminView: {}, PermissionUsersRead: {}, PermissionUsersManage: {},
		PermissionChannelsManage: {}, PermissionBalancesManage: {}, PermissionRolesManage: {}, PermissionRegistrationManage: {},
	},
	RoleOperator: {
		PermissionAdminView: {}, PermissionUsersRead: {}, PermissionUsersManage: {}, PermissionBalancesManage: {},
	},
	RoleChannelManager: {
		PermissionAdminView: {}, PermissionChannelsManage: {}, PermissionBalancesManage: {},
	},
	RolePlayer: {},
}

func ValidRole(role string) bool {
	_, ok := rolePermissions[Role(role)]
	return ok
}

func Has(role string, permission Permission) bool {
	permissions, ok := rolePermissions[Role(role)]
	if !ok {
		return false
	}
	_, ok = permissions[permission]
	return ok
}

func Permissions(role string) []Permission {
	granted := rolePermissions[Role(role)]
	result := make([]Permission, 0, len(granted))
	for _, definition := range permissionCatalog {
		if _, ok := granted[definition.Key]; ok {
			result = append(result, definition.Key)
		}
	}
	return result
}

func AllPermissions() []Permission {
	result := make([]Permission, 0, len(permissionCatalog))
	for _, definition := range permissionCatalog {
		result = append(result, definition.Key)
	}
	return result
}

func Catalog() []PermissionDefinition {
	result := make([]PermissionDefinition, len(permissionCatalog))
	copy(result, permissionCatalog)
	return result
}

func ValidPermission(permission string) bool {
	for _, definition := range permissionCatalog {
		if string(definition.Key) == permission {
			return true
		}
	}
	return false
}

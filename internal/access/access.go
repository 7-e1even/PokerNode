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
	PermissionRankingsManage     Permission = "rankings:manage"
	PermissionAuthSettingsManage Permission = "auth_settings:manage"
	PermissionBrandingManage     Permission = "branding:manage"
)

type PermissionDefinition struct {
	Key         Permission   `json:"key"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Group       string       `json:"group"`
	Requires    []Permission `json:"requires,omitempty"`
}

var permissionCatalog = []PermissionDefinition{
	{Key: PermissionAdminView, Name: "访问运营后台", Description: "登录并查看运营后台", Group: "后台访问"},
	{Key: PermissionUsersRead, Name: "查看用户", Description: "查看平台用户与账号状态", Group: "用户管理", Requires: []Permission{PermissionAdminView}},
	{Key: PermissionUsersManage, Name: "管理用户", Description: "创建、编辑、停用和删除允许管理的账号", Group: "用户管理", Requires: []Permission{PermissionUsersRead}},
	{Key: PermissionChannelsManage, Name: "管理频道", Description: "管理分配频道的连接、牌桌和邀请码", Group: "频道管理", Requires: []Permission{PermissionAdminView}},
	{Key: PermissionBalancesManage, Name: "管理余额", Description: "调整分配频道内成员的 New API 余额", Group: "频道管理", Requires: []Permission{PermissionAdminView}},
	{Key: PermissionRankingsManage, Name: "管理排名", Description: "管理排行榜展示和净胜金额", Group: "运营管理", Requires: []Permission{PermissionAdminView}},
	{Key: PermissionRolesManage, Name: "管理角色", Description: "创建角色并配置不超过自身范围的权限", Group: "权限管理", Requires: []Permission{PermissionAdminView}},
	{Key: PermissionRegistrationManage, Name: "管理注册策略", Description: "开启或关闭平台注册入口", Group: "系统设置", Requires: []Permission{PermissionAdminView}},
	{Key: PermissionAuthSettingsManage, Name: "管理登录配置", Description: "修改微信登录等身份认证设置", Group: "系统设置", Requires: []Permission{PermissionAdminView}},
	{Key: PermissionBrandingManage, Name: "管理站点品牌", Description: "修改站点名称、浏览器图标和登录页展示", Group: "系统设置", Requires: []Permission{PermissionAdminView}},
}

var rolePermissions = map[Role]map[Permission]struct{}{
	RoleSuperAdmin: {
		PermissionAdminView: {}, PermissionUsersRead: {}, PermissionUsersManage: {},
		PermissionChannelsManage: {}, PermissionBalancesManage: {}, PermissionRolesManage: {}, PermissionRegistrationManage: {},
		PermissionRankingsManage: {}, PermissionAuthSettingsManage: {}, PermissionBrandingManage: {},
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

func ExpandPermissions(permissions []Permission) []Permission {
	granted := make(map[Permission]struct{}, len(permissions))
	for _, permission := range permissions {
		granted[permission] = struct{}{}
	}
	for changed := true; changed; {
		changed = false
		for _, definition := range permissionCatalog {
			if _, ok := granted[definition.Key]; !ok {
				continue
			}
			for _, required := range definition.Requires {
				if _, ok := granted[required]; ok {
					continue
				}
				granted[required] = struct{}{}
				changed = true
			}
		}
	}
	result := make([]Permission, 0, len(granted))
	for _, definition := range permissionCatalog {
		if _, ok := granted[definition.Key]; ok {
			result = append(result, definition.Key)
		}
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

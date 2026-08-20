package app

import (
	"context"

	"pokernode/internal/access"
	"pokernode/internal/store"
)

func (s *Server) permissionsForUser(ctx context.Context, user store.User) ([]access.Permission, error) {
	if user.Role == string(access.RoleSuperAdmin) {
		return access.AllPermissions(), nil
	}
	stored, err := s.store.PermissionsForRole(ctx, user.Role)
	if err != nil {
		return nil, err
	}
	permissions := make([]access.Permission, 0, len(stored))
	seen := make(map[access.Permission]struct{}, len(stored))
	for _, value := range stored {
		if !access.ValidPermission(value) {
			continue
		}
		permission := access.Permission(value)
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		permissions = append(permissions, permission)
	}
	return access.ExpandPermissions(permissions), nil
}

func (s *Server) hasPermission(ctx context.Context, user store.User, permission access.Permission) (bool, error) {
	permissions, err := s.permissionsForUser(ctx, user)
	if err != nil {
		return false, err
	}
	for _, granted := range permissions {
		if granted == permission {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) spaceForActor(ctx context.Context, spaceID string, user store.User) (store.Space, error) {
	canManageAssigned, err := s.hasPermission(ctx, user, access.PermissionChannelsManage)
	if err != nil {
		return store.Space{}, err
	}
	return s.store.SpaceForActor(ctx, spaceID, user.ID, canManageAssigned, user.Role == string(access.RoleSuperAdmin))
}

func (s *Server) spacesForActor(ctx context.Context, user store.User) ([]store.Space, error) {
	canManageAssigned, err := s.hasPermission(ctx, user, access.PermissionChannelsManage)
	if err != nil {
		return nil, err
	}
	return s.store.SpacesForActor(ctx, user.ID, canManageAssigned, user.Role == string(access.RoleSuperAdmin))
}

func (s *Server) canManageAssignedSpace(ctx context.Context, user store.User, spaceID string, permission access.Permission) (bool, error) {
	if user.Role == string(access.RoleSuperAdmin) {
		return true, nil
	}
	allowed, err := s.hasPermission(ctx, user, permission)
	if err != nil || !allowed {
		return false, err
	}
	return s.store.IsSpaceManager(ctx, spaceID, user.ID)
}

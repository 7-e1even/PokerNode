package app

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"pokernode/internal/access"
	"pokernode/internal/store"
)

var roleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`)

type roleInput struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (s *Server) handleAdminRoles(w http.ResponseWriter, r *http.Request, _ store.User) error {
	roles, err := s.store.Roles(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles, "permission_catalog": access.Catalog()})
	return nil
}

func (s *Server) handleAdminCreateRole(w http.ResponseWriter, r *http.Request, _ store.User) error {
	var input roleInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	role, err := validateRoleInput(input, true)
	if err != nil {
		return err
	}
	created, err := s.store.CreateRole(r.Context(), role)
	if err != nil {
		if store.IsUniqueViolation(err) {
			return &apiError{Status: http.StatusConflict, Message: "角色标识已存在"}
		}
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"role": created})
	return nil
}

func (s *Server) handleAdminUpdateRole(w http.ResponseWriter, r *http.Request, _ store.User) error {
	key := strings.TrimSpace(r.PathValue("roleKey"))
	if !roleKeyPattern.MatchString(key) {
		return &apiError{Status: http.StatusBadRequest, Message: "角色标识无效"}
	}
	var input roleInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Key = key
	role, err := validateRoleInput(input, false)
	if err != nil {
		return err
	}
	updated, err := s.store.UpdateRole(r.Context(), key, role.Name, role.Description, role.Permissions)
	if err != nil {
		if errors.Is(err, store.ErrSystemRole) {
			return &apiError{Status: http.StatusConflict, Message: "系统内置角色不可修改"}
		}
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"role": updated})
	return nil
}

func (s *Server) handleAdminDeleteRole(w http.ResponseWriter, r *http.Request, _ store.User) error {
	key := strings.TrimSpace(r.PathValue("roleKey"))
	if err := s.store.DeleteRole(r.Context(), key); err != nil {
		switch {
		case errors.Is(err, store.ErrSystemRole):
			return &apiError{Status: http.StatusConflict, Message: "系统内置角色不可删除"}
		case errors.Is(err, store.ErrRoleReferenced):
			return &apiError{Status: http.StatusConflict, Message: "该角色仍有用户使用，请先调整用户角色"}
		default:
			return err
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func validateRoleInput(input roleInput, requireKey bool) (store.Role, error) {
	input.Key = strings.TrimSpace(input.Key)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if requireKey && !roleKeyPattern.MatchString(input.Key) {
		return store.Role{}, &apiError{Status: http.StatusBadRequest, Message: "角色标识需为 3–32 位小写字母、数字或下划线，且以字母开头"}
	}
	if input.Name == "" || len([]rune(input.Name)) > 32 {
		return store.Role{}, &apiError{Status: http.StatusBadRequest, Message: "角色名称需为 1–32 个字符"}
	}
	if len([]rune(input.Description)) > 200 {
		return store.Role{}, &apiError{Status: http.StatusBadRequest, Message: "角色说明不能超过 200 个字符"}
	}
	permissions := make([]string, 0, len(input.Permissions))
	seen := make(map[string]struct{}, len(input.Permissions))
	for _, permission := range input.Permissions {
		permission = strings.TrimSpace(permission)
		if !access.ValidPermission(permission) {
			return store.Role{}, &apiError{Status: http.StatusBadRequest, Message: "包含未知的功能权限"}
		}
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		permissions = append(permissions, permission)
	}
	return store.Role{Key: input.Key, Name: input.Name, Description: input.Description, Permissions: permissions}, nil
}

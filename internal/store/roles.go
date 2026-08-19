package store

import (
	"context"
	"encoding/json"
	"time"
)

type Role struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	System      bool     `json:"system"`
	UserCount   int      `json:"user_count"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

func (s *Store) Roles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.key,r.name,r.description,r.permissions_json,r.system,
(SELECT COUNT(*) FROM users u WHERE u.role=r.key),r.created_at,r.updated_at
FROM roles r ORDER BY r.system DESC,
CASE r.key WHEN 'super_admin' THEN 0 WHEN 'channel_manager' THEN 1 WHEN 'operator' THEN 2 WHEN 'player' THEN 3 ELSE 4 END,
r.created_at,r.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]Role, 0)
	for rows.Next() {
		var role Role
		var permissionsJSON string
		if err := rows.Scan(&role.Key, &role.Name, &role.Description, &permissionsJSON, &role.System, &role.UserCount, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(permissionsJSON), &role.Permissions); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (s *Store) RoleByKey(ctx context.Context, key string) (Role, error) {
	var role Role
	var permissionsJSON string
	err := s.db.QueryRowContext(ctx, `SELECT r.key,r.name,r.description,r.permissions_json,r.system,
(SELECT COUNT(*) FROM users u WHERE u.role=r.key),r.created_at,r.updated_at FROM roles r WHERE r.key=$1`, key).
		Scan(&role.Key, &role.Name, &role.Description, &permissionsJSON, &role.System, &role.UserCount, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		return Role{}, mapNotFound(err)
	}
	if err := json.Unmarshal([]byte(permissionsJSON), &role.Permissions); err != nil {
		return Role{}, err
	}
	return role, nil
}

func (s *Store) CreateRole(ctx context.Context, role Role) (Role, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	permissionsJSON, err := json.Marshal(role.Permissions)
	if err != nil {
		return Role{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO roles(key,name,description,permissions_json,system,created_at,updated_at) VALUES($1,$2,$3,$4,FALSE,$5,$6)`,
		role.Key, role.Name, role.Description, string(permissionsJSON), now, now)
	if err != nil {
		return Role{}, err
	}
	return s.RoleByKey(ctx, role.Key)
}

func (s *Store) UpdateRole(ctx context.Context, key, name, description string, permissions []string) (Role, error) {
	var system bool
	if err := s.db.QueryRowContext(ctx, `SELECT system FROM roles WHERE key=$1`, key).Scan(&system); err != nil {
		return Role{}, mapNotFound(err)
	}
	if system {
		return Role{}, ErrSystemRole
	}
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return Role{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE roles SET name=$1,description=$2,permissions_json=$3,updated_at=$4 WHERE key=$5`,
		name, description, string(permissionsJSON), time.Now().UTC().Format(time.RFC3339Nano), key)
	if err != nil {
		return Role{}, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return Role{}, ErrNotFound
	}
	return s.RoleByKey(ctx, key)
}

func (s *Store) DeleteRole(ctx context.Context, key string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var system bool
	if err := tx.QueryRowContext(ctx, `SELECT system FROM roles WHERE key=$1`, key).Scan(&system); err != nil {
		return mapNotFound(err)
	}
	if system {
		return ErrSystemRole
	}
	var users int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role=$1`, key).Scan(&users); err != nil {
		return err
	}
	if users > 0 {
		return ErrRoleReferenced
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE key=$1`, key); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PermissionsForRole(ctx context.Context, key string) ([]string, error) {
	role, err := s.RoleByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	return role.Permissions, nil
}

func (s *Store) ManagedSpaceIDs(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sm.space_id
FROM space_managers sm
JOIN space_members m ON m.space_id=sm.space_id AND m.user_id=sm.user_id
WHERE sm.user_id=$1 ORDER BY sm.created_at,sm.space_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spaceIDs := make([]string, 0)
	for rows.Next() {
		var spaceID string
		if err := rows.Scan(&spaceID); err != nil {
			return nil, err
		}
		spaceIDs = append(spaceIDs, spaceID)
	}
	return spaceIDs, rows.Err()
}

func (s *Store) JoinedSpaceIDs(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT space_id FROM space_members WHERE user_id=$1 ORDER BY space_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spaceIDs := make([]string, 0)
	for rows.Next() {
		var spaceID string
		if err := rows.Scan(&spaceID); err != nil {
			return nil, err
		}
		spaceIDs = append(spaceIDs, spaceID)
	}
	return spaceIDs, rows.Err()
}

func (s *Store) SetManagedSpaces(ctx context.Context, userID int64, spaceIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, userID).Scan(&userExists); err != nil {
		return err
	}
	if !userExists {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM space_managers WHERE user_id=$1`, userID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	seen := make(map[string]struct{}, len(spaceIDs))
	for _, spaceID := range spaceIDs {
		if _, ok := seen[spaceID]; ok {
			continue
		}
		seen[spaceID] = struct{}{}
		result, err := tx.ExecContext(ctx, `INSERT INTO space_managers(space_id,user_id,created_at)
SELECT space_id,$1,$2 FROM space_members WHERE space_id=$3 AND user_id=$1`, userID, now, spaceID)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return ErrSpaceMembershipRequired
		}
	}
	return tx.Commit()
}

func (s *Store) ValidateManagedSpaceIDs(ctx context.Context, userID int64, spaceIDs []string) error {
	seen := make(map[string]struct{}, len(spaceIDs))
	for _, spaceID := range spaceIDs {
		if _, ok := seen[spaceID]; ok {
			continue
		}
		seen[spaceID] = struct{}{}
		var joined bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM space_members WHERE space_id=$1 AND user_id=$2)`, spaceID, userID).Scan(&joined); err != nil {
			return err
		}
		if !joined {
			return ErrSpaceMembershipRequired
		}
	}
	return nil
}

func (s *Store) ValidateSpaceIDs(ctx context.Context, spaceIDs []string) error {
	seen := make(map[string]struct{}, len(spaceIDs))
	for _, spaceID := range spaceIDs {
		if _, ok := seen[spaceID]; ok {
			continue
		}
		seen[spaceID] = struct{}{}
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM spaces WHERE id=$1)`, spaceID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) IsSpaceManager(ctx context.Context, spaceID string, userID int64) (bool, error) {
	var assigned bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM space_managers sm
JOIN space_members m ON m.space_id=sm.space_id AND m.user_id=sm.user_id
WHERE sm.space_id=$1 AND sm.user_id=$2)`, spaceID, userID).Scan(&assigned)
	return assigned, err
}

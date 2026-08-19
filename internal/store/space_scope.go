package store

import "context"

func (s *Store) SpacesForActor(ctx context.Context, userID int64, includeAssigned, includeAll bool) ([]Space, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.name,s.invite_code,s.owner_user_id,s.newapi_base_url,s.admin_token_last4,
s.admin_newapi_user_id,s.admin_newapi_role,s.quota_per_usd,s.created_at,
(s.owner_user_id=$1),
(m.newapi_user_id IS NOT NULL),
($2 OR s.owner_user_id=$1 OR ($3 AND sm.user_id IS NOT NULL AND m.user_id IS NOT NULL))
FROM spaces s
LEFT JOIN space_members m ON m.space_id=s.id AND m.user_id=$1
LEFT JOIN space_managers sm ON sm.space_id=s.id AND sm.user_id=$1
WHERE $2 OR m.user_id IS NOT NULL OR s.owner_user_id=$1 OR ($3 AND sm.user_id IS NOT NULL AND m.user_id IS NOT NULL)
ORDER BY s.created_at DESC`, userID, includeAll, includeAssigned)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spaces := make([]Space, 0)
	for rows.Next() {
		var space Space
		if err := rows.Scan(&space.ID, &space.Name, &space.InviteCode, &space.OwnerUserID, &space.BaseURL, &space.AdminTokenLast4,
			&space.AdminNewAPIUserID, &space.AdminNewAPIRole, &space.QuotaPerUSD, &space.CreatedAt, &space.IsOwner, &space.IsBound, &space.CanManage); err != nil {
			return nil, err
		}
		if !space.CanManage {
			space.InviteCode = ""
		}
		spaces = append(spaces, space)
	}
	return spaces, rows.Err()
}

func (s *Store) SpaceForActor(ctx context.Context, spaceID string, userID int64, includeAssigned, includeAll bool) (Space, error) {
	var space Space
	err := s.db.QueryRowContext(ctx, `SELECT s.id,s.name,s.invite_code,s.owner_user_id,s.newapi_base_url,s.admin_token_enc,s.admin_token_last4,
s.admin_newapi_user_id,s.admin_newapi_role,s.quota_per_usd,s.created_at,
(s.owner_user_id=$1),
(m.newapi_user_id IS NOT NULL),
($2 OR s.owner_user_id=$1 OR ($3 AND sm.user_id IS NOT NULL AND m.user_id IS NOT NULL))
FROM spaces s
LEFT JOIN space_members m ON m.space_id=s.id AND m.user_id=$1
LEFT JOIN space_managers sm ON sm.space_id=s.id AND sm.user_id=$1
WHERE s.id=$4 AND ($2 OR m.user_id IS NOT NULL OR s.owner_user_id=$1 OR ($3 AND sm.user_id IS NOT NULL AND m.user_id IS NOT NULL))`,
		userID, includeAll, includeAssigned, spaceID).
		Scan(&space.ID, &space.Name, &space.InviteCode, &space.OwnerUserID, &space.BaseURL, &space.AdminTokenEnc, &space.AdminTokenLast4,
			&space.AdminNewAPIUserID, &space.AdminNewAPIRole, &space.QuotaPerUSD, &space.CreatedAt, &space.IsOwner, &space.IsBound, &space.CanManage)
	if err == nil && !space.CanManage {
		space.InviteCode = ""
	}
	return space, mapNotFound(err)
}

func (s *Store) UpdateSpaceConnectionManaged(ctx context.Context, spaceID, baseURL, tokenEnc, last4 string, adminUserID int64, adminRole int, quotaPerUSD int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE spaces SET newapi_base_url=$1,admin_token_enc=$2,admin_token_last4=$3,admin_newapi_user_id=$4,admin_newapi_role=$5,quota_per_usd=$6 WHERE id=$7`,
		baseURL, tokenEnc, last4, adminUserID, adminRole, quotaPerUSD, spaceID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AdminSpacesForUser(ctx context.Context, userID int64, includeAssigned bool) ([]AdminSpaceSummary, AdminPlatformCounts, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
s.id,s.name,u.username,u.display_name,s.newapi_base_url,s.admin_token_last4,s.quota_per_usd,s.created_at,
(SELECT COUNT(*) FROM space_members m WHERE m.space_id=s.id),
(SELECT COUNT(*) FROM space_members m WHERE m.space_id=s.id AND m.newapi_user_id IS NOT NULL),
(SELECT COUNT(*) FROM table_states t WHERE t.space_id=s.id),
(SELECT COUNT(*) FROM wallet_operations w WHERE w.space_id=s.id),
(SELECT COUNT(*) FROM wallet_operations w WHERE w.space_id=s.id AND w.status='failed')
FROM spaces s JOIN users u ON u.id=s.owner_user_id
LEFT JOIN space_managers sm ON sm.space_id=s.id AND sm.user_id=$1
LEFT JOIN space_members m ON m.space_id=s.id AND m.user_id=$1
WHERE s.owner_user_id=$1 OR ($2 AND sm.user_id IS NOT NULL AND m.user_id IS NOT NULL)
ORDER BY s.created_at DESC`, userID, includeAssigned)
	if err != nil {
		return nil, AdminPlatformCounts{}, err
	}
	defer rows.Close()
	return scanAdminSpaces(rows)
}

func scanAdminSpaces(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]AdminSpaceSummary, AdminPlatformCounts, error) {
	spaces := make([]AdminSpaceSummary, 0)
	var counts AdminPlatformCounts
	for rows.Next() {
		var space AdminSpaceSummary
		if err := rows.Scan(&space.ID, &space.Name, &space.OwnerUsername, &space.OwnerDisplayName, &space.BaseURL,
			&space.AdminTokenLast4, &space.QuotaPerUSD, &space.CreatedAt, &space.MemberCount, &space.BoundMemberCount,
			&space.TableCount, &space.OperationCount, &space.FailedOperations); err != nil {
			return nil, AdminPlatformCounts{}, err
		}
		spaces = append(spaces, space)
		counts.Spaces++
		counts.Memberships += space.MemberCount
		counts.BoundMemberships += space.BoundMemberCount
		counts.Tables += space.TableCount
		counts.Operations += space.OperationCount
		counts.FailedOperations += space.FailedOperations
	}
	if err := rows.Err(); err != nil {
		return nil, AdminPlatformCounts{}, err
	}
	return spaces, counts, nil
}

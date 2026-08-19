package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrNotFound                = errors.New("not found")
	ErrRegistrationClosed      = errors.New("registration closed")
	ErrLastSuperAdmin          = errors.New("last active super administrator")
	ErrUserReferenced          = errors.New("user has business records")
	ErrSystemRole              = errors.New("system role cannot be changed")
	ErrRoleReferenced          = errors.New("role is assigned to users")
	ErrSpaceMembershipRequired = errors.New("space membership required")
	ErrExternalIdentityBound   = errors.New("external identity is bound to another user")
	ErrExternalIdentitySet     = errors.New("user already has an external identity for this provider")
)

const externalLoginPasswordHash = "!external-login"

type Store struct {
	db *sql.DB
}

type User struct {
	ID              int64    `json:"id"`
	Username        string   `json:"username"`
	DisplayName     string   `json:"display_name"`
	PasswordHash    string   `json:"-"`
	Role            string   `json:"role"`
	RoleName        string   `json:"role_name"`
	Status          string   `json:"status"`
	CreatedAt       string   `json:"created_at"`
	RankingHidden   bool     `json:"ranking_hidden"`
	ManagedSpaceIDs []string `json:"managed_space_ids,omitempty"`
	JoinedSpaceIDs  []string `json:"joined_space_ids,omitempty"`
}

func (u User) HasPassword() bool {
	return u.PasswordHash != externalLoginPasswordHash
}

type Space struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	InviteCode        string `json:"invite_code,omitempty"`
	OwnerUserID       int64  `json:"owner_user_id"`
	BaseURL           string `json:"newapi_base_url"`
	AdminTokenEnc     string `json:"-"`
	AdminTokenLast4   string `json:"admin_token_last4"`
	AdminNewAPIUserID int64  `json:"admin_newapi_user_id"`
	AdminNewAPIRole   int    `json:"admin_newapi_role"`
	QuotaPerUSD       int64  `json:"quota_per_usd"`
	CreatedAt         string `json:"created_at"`
	IsOwner           bool   `json:"is_owner"`
	IsBound           bool   `json:"is_bound"`
	CanManage         bool   `json:"can_manage"`
}

type Member struct {
	SpaceID          string `json:"space_id"`
	UserID           int64  `json:"user_id"`
	NewAPIUserID     int64  `json:"newapi_user_id,omitempty"`
	NewAPIUsername   string `json:"newapi_username,omitempty"`
	NewAPIDisplay    string `json:"newapi_display_name,omitempty"`
	NewAPIRole       int    `json:"newapi_role,omitempty"`
	UserTokenEnc     string `json:"-"`
	UserTokenLast4   string `json:"user_token_last4,omitempty"`
	VerifiedAt       string `json:"verified_at,omitempty"`
	PokerDisplayName string `json:"poker_display_name"`
}

type WalletOperation struct {
	ID           string `json:"id"`
	SpaceID      string `json:"space_id"`
	TableID      string `json:"table_id"`
	UserID       int64  `json:"user_id"`
	NewAPIUserID int64  `json:"newapi_user_id"`
	ActorUserID  int64  `json:"actor_user_id,omitempty"`
	Kind         string `json:"kind"`
	Cents        int64  `json:"cents"`
	Quota        int64  `json:"quota"`
	Note         string `json:"note,omitempty"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type ChannelLeaderboardEntry struct {
	UserID      int64  `json:"user_id"`
	DisplayName string `json:"display_name"`
	NetCents    int64  `json:"net_cents"`
	Sessions    int    `json:"sessions"`
}

type AdminSpaceSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	OwnerUsername    string `json:"owner_username"`
	OwnerDisplayName string `json:"owner_display_name"`
	BaseURL          string `json:"newapi_base_url"`
	AdminTokenLast4  string `json:"admin_token_last4"`
	QuotaPerUSD      int64  `json:"quota_per_usd"`
	CreatedAt        string `json:"created_at"`
	MemberCount      int    `json:"member_count"`
	BoundMemberCount int    `json:"bound_member_count"`
	TableCount       int    `json:"table_count"`
	OperationCount   int    `json:"operation_count"`
	FailedOperations int    `json:"failed_operations"`
}

type AdminPlatformCounts struct {
	Spaces           int `json:"spaces"`
	Memberships      int `json:"memberships"`
	BoundMemberships int `json:"bound_memberships"`
	Tables           int `json:"tables"`
	Operations       int `json:"operations"`
	FailedOperations int `json:"failed_operations"`
}

func Open(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS roles (
  key TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  permissions_json TEXT NOT NULL DEFAULT '[]',
  system BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  username TEXT NOT NULL,
  display_name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'player',
  status TEXT NOT NULL DEFAULT 'active',
  ranking_hidden BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users(LOWER(username));
CREATE TABLE IF NOT EXISTS auth_identities (
  provider TEXT NOT NULL,
  subject TEXT NOT NULL,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (provider, subject),
  UNIQUE (provider, user_id)
);
CREATE INDEX IF NOT EXISTS auth_identities_user_idx ON auth_identities(user_id, provider);
CREATE TABLE IF NOT EXISTS app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS spaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  invite_code TEXT NOT NULL,
  owner_user_id BIGINT NOT NULL REFERENCES users(id),
  newapi_base_url TEXT NOT NULL,
  admin_token_enc TEXT NOT NULL,
  admin_token_last4 TEXT NOT NULL,
  admin_newapi_user_id BIGINT NOT NULL,
  admin_newapi_role INTEGER NOT NULL,
  quota_per_usd BIGINT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS spaces_invite_code_lower_idx ON spaces(LOWER(invite_code));
CREATE TABLE IF NOT EXISTS space_members (
  space_id TEXT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  newapi_user_id BIGINT,
  newapi_username TEXT,
  newapi_display_name TEXT,
  newapi_role INTEGER,
  user_token_enc TEXT,
  user_token_last4 TEXT,
  verified_at TEXT,
  PRIMARY KEY (space_id, user_id),
  UNIQUE (space_id, newapi_user_id)
);
CREATE TABLE IF NOT EXISTS space_managers (
  space_id TEXT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (space_id, user_id)
);
CREATE INDEX IF NOT EXISTS space_managers_user_idx ON space_managers(user_id, space_id);
CREATE TABLE IF NOT EXISTS wallet_operations (
  id TEXT PRIMARY KEY,
  space_id TEXT NOT NULL REFERENCES spaces(id),
  table_id TEXT NOT NULL DEFAULT '',
  user_id BIGINT NOT NULL REFERENCES users(id),
  newapi_user_id BIGINT NOT NULL,
  actor_user_id BIGINT NOT NULL DEFAULT 0,
  kind TEXT NOT NULL,
  cents BIGINT NOT NULL,
  quota BIGINT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS wallet_operations_user_idx ON wallet_operations(space_id, user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS table_states (
  space_id TEXT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  table_id TEXT NOT NULL,
  state_json BYTEA NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (space_id, table_id)
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if err := s.ensureColumn("users", "role", "TEXT NOT NULL DEFAULT 'player'"); err != nil {
		return err
	}
	if err := s.ensureColumn("users", "status", "TEXT NOT NULL DEFAULT 'active'"); err != nil {
		return err
	}
	if err := s.ensureColumn("users", "ranking_hidden", "BOOLEAN NOT NULL DEFAULT FALSE"); err != nil {
		return err
	}
	if err := s.ensureColumn("wallet_operations", "table_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("wallet_operations", "actor_user_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("wallet_operations", "note", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`INSERT INTO app_settings(key,value,updated_at) VALUES('registration_enabled','true',$1) ON CONFLICT(key) DO NOTHING`, now); err != nil {
		return fmt.Errorf("initialize settings: %w", err)
	}
	defaultRoles := []struct {
		key, name, description, permissions string
		system                              bool
	}{
		{"super_admin", "超级管理员", "拥有平台和全部频道的所有权限", `["admin:view","users:read","users:manage","channels:manage","balances:manage","roles:manage","registration:manage"]`, true},
		{"operator", "运营", "管理玩家账号并处理已分配频道的成员余额", `["admin:view","users:read","users:manage","balances:manage"]`, false},
		{"channel_manager", "频道管理员", "管理分配给自己的一个或多个频道", `["admin:view","channels:manage","balances:manage"]`, false},
		{"player", "玩家", "进入频道并参与牌局", `[]`, false},
	}
	var defaultRolesSeeded bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM app_settings WHERE key='default_roles_seeded')`).Scan(&defaultRolesSeeded); err != nil {
		return fmt.Errorf("check default roles: %w", err)
	}
	if !defaultRolesSeeded {
		for _, role := range defaultRoles {
			if _, err := s.db.Exec(`INSERT INTO roles(key,name,description,permissions_json,system,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(key) DO NOTHING`, role.key, role.name, role.description, role.permissions, role.system, now, now); err != nil {
				return fmt.Errorf("initialize role %s: %w", role.key, err)
			}
		}
		if _, err := s.db.Exec(`INSERT INTO app_settings(key,value,updated_at) VALUES('default_roles_seeded','true',$1)`, now); err != nil {
			return fmt.Errorf("mark default roles initialized: %w", err)
		}
	}
	adminRole := defaultRoles[0]
	if _, err := s.db.Exec(`INSERT INTO roles(key,name,description,permissions_json,system,created_at,updated_at) VALUES($1,$2,$3,$4,TRUE,$5,$6) ON CONFLICT(key) DO NOTHING`, adminRole.key, adminRole.name, adminRole.description, adminRole.permissions, now, now); err != nil {
		return fmt.Errorf("initialize super administrator role: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE roles SET system=(key='super_admin')`); err != nil {
		return fmt.Errorf("normalize system roles: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE users SET role='super_admin' WHERE id=(SELECT MIN(id) FROM users) AND NOT EXISTS(SELECT 1 FROM users WHERE role='super_admin')`); err != nil {
		return fmt.Errorf("bootstrap administrator: %w", err)
	}
	return nil
}

func (s *Store) ensureColumn(table, column, definition string) error {
	if _, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN IF NOT EXISTS ` + column + ` ` + definition); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *Store) CreateRegisteredUser(ctx context.Context, username, displayName, passwordHash string) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return User{}, err
	}
	var registrationEnabled string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key='registration_enabled'`).Scan(&registrationEnabled); err != nil {
		return User{}, err
	}
	if count > 0 && registrationEnabled != "true" {
		return User{}, ErrRegistrationClosed
	}
	role := "player"
	if count == 0 {
		role = "super_admin"
	}
	user, err := createUser(ctx, tx, username, displayName, passwordHash, role)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) CreateUser(ctx context.Context, username, displayName, passwordHash, role string) (User, error) {
	return createUser(ctx, s.db, username, displayName, passwordHash, role)
}

func (s *Store) UserByExternalIdentity(ctx context.Context, provider, subject string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.display_name,u.password_hash,u.role,COALESCE(r.name,u.role),u.status,u.created_at,u.ranking_hidden
FROM auth_identities i JOIN users u ON u.id=i.user_id LEFT JOIN roles r ON r.key=u.role
WHERE i.provider=$1 AND i.subject=$2`, provider, subject).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &user.Role, &user.RoleName, &user.Status, &user.CreatedAt, &user.RankingHidden)
	return user, mapNotFound(err)
}

func (s *Store) HasExternalIdentity(ctx context.Context, userID int64, provider string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_identities WHERE user_id=$1 AND provider=$2)`, userID, provider).Scan(&exists)
	return exists, err
}

func (s *Store) CreateExternalUser(ctx context.Context, provider, subject, displayName string) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var existingUserID int64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM auth_identities WHERE provider=$1 AND subject=$2`, provider, subject).Scan(&existingUserID)
	if err == nil {
		return userByID(ctx, tx, existingUserID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return User{}, err
	}

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return User{}, err
	}
	var registrationEnabled string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key='registration_enabled'`).Scan(&registrationEnabled); err != nil {
		return User{}, err
	}
	if count > 0 && registrationEnabled != "true" {
		return User{}, ErrRegistrationClosed
	}

	role := "player"
	if count == 0 {
		role = "super_admin"
	}
	username, err := availableExternalUsername(ctx, tx, provider, subject)
	if err != nil {
		return User{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "微信用户"
	}
	if characters := []rune(displayName); len(characters) > 32 {
		displayName = string(characters[:32])
	}
	user, err := createUser(ctx, tx, username, displayName, externalLoginPasswordHash, role)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO auth_identities(provider,subject,user_id,created_at) VALUES($1,$2,$3,$4)`, provider, subject, user.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) BindExternalIdentity(ctx context.Context, userID int64, provider, subject string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var boundUserID int64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM auth_identities WHERE provider=$1 AND subject=$2`, provider, subject).Scan(&boundUserID)
	if err == nil {
		if boundUserID == userID {
			return nil
		}
		return ErrExternalIdentityBound
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var existingSubject string
	err = tx.QueryRowContext(ctx, `SELECT subject FROM auth_identities WHERE provider=$1 AND user_id=$2`, provider, userID).Scan(&existingSubject)
	if err == nil {
		return ErrExternalIdentitySet
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO auth_identities(provider,subject,user_id,created_at) VALUES($1,$2,$3,$4)`, provider, subject, userID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func availableExternalUsername(ctx context.Context, tx *sql.Tx, provider, subject string) (string, error) {
	digest := sha256.Sum256([]byte(provider + "\x00" + subject))
	base := "wx_" + hex.EncodeToString(digest[:8])
	for suffix := 1; suffix <= 99; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s_%d", base, suffix)
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(username)=LOWER($1))`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", errors.New("generate external username")
}

func createUser(ctx context.Context, queryer sqlQueryer, username, displayName, passwordHash, role string) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var id int64
	if err := queryer.QueryRowContext(ctx, `INSERT INTO users(username,display_name,password_hash,role,status,created_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, username, displayName, passwordHash, role, "active", now).Scan(&id); err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, DisplayName: displayName, PasswordHash: passwordHash, Role: role, RoleName: role, Status: "active", CreatedAt: now}, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.display_name,u.password_hash,u.role,COALESCE(r.name,u.role),u.status,u.created_at,u.ranking_hidden FROM users u LEFT JOIN roles r ON r.key=u.role WHERE LOWER(u.username)=LOWER($1)`, username).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &user.Role, &user.RoleName, &user.Status, &user.CreatedAt, &user.RankingHidden)
	return user, mapNotFound(err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return userByID(ctx, s.db, id)
}

type sqlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func userByID(ctx context.Context, queryer sqlQueryer, id int64) (User, error) {
	var user User
	err := queryer.QueryRowContext(ctx, `SELECT u.id,u.username,u.display_name,u.password_hash,u.role,COALESCE(r.name,u.role),u.status,u.created_at,u.ranking_hidden FROM users u LEFT JOIN roles r ON r.key=u.role WHERE u.id=$1`, id).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &user.Role, &user.RoleName, &user.Status, &user.CreatedAt, &user.RankingHidden)
	return user, mapNotFound(err)
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id,u.username,u.display_name,u.password_hash,u.role,COALESCE(r.name,u.role),u.status,u.created_at,u.ranking_hidden FROM users u LEFT JOIN roles r ON r.key=u.role ORDER BY u.created_at DESC`)
	if err != nil {
		return nil, err
	}
	users := make([]User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &user.Role, &user.RoleName, &user.Status, &user.CreatedAt, &user.RankingHidden); err != nil {
			rows.Close()
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range users {
		managedSpaceIDs, err := s.ManagedSpaceIDs(ctx, users[index].ID)
		if err != nil {
			return nil, err
		}
		users[index].ManagedSpaceIDs = managedSpaceIDs
		joinedSpaceIDs, err := s.JoinedSpaceIDs(ctx, users[index].ID)
		if err != nil {
			return nil, err
		}
		users[index].JoinedSpaceIDs = joinedSpaceIDs
	}
	return users, nil
}

func (s *Store) UpdateUser(ctx context.Context, userID int64, username, displayName, passwordHash, role, status string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentRole, currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT role,status FROM users WHERE id=$1`, userID).Scan(&currentRole, &currentStatus); err != nil {
		return mapNotFound(err)
	}
	if currentRole == "super_admin" && currentStatus == "active" && (role != "super_admin" || status != "active") {
		var activeAdmins int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role='super_admin' AND status='active'`).Scan(&activeAdmins); err != nil {
			return err
		}
		if activeAdmins <= 1 {
			return ErrLastSuperAdmin
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE users SET username=$1,display_name=$2,password_hash=$3,role=$4,status=$5 WHERE id=$6`, username, displayName, passwordHash, role, status, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) UpdateUserCredentials(ctx context.Context, userID int64, username, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET username=$1,password_hash=$2 WHERE id=$3`, username, passwordHash, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetUserRankingHidden(ctx context.Context, userID int64, hidden bool) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET ranking_hidden=$1 WHERE id=$2`, hidden, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var role, status string
	if err := tx.QueryRowContext(ctx, `SELECT role,status FROM users WHERE id=$1`, userID).Scan(&role, &status); err != nil {
		return mapNotFound(err)
	}
	if role == "super_admin" && status == "active" {
		var activeAdmins int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role='super_admin' AND status='active'`).Scan(&activeAdmins); err != nil {
			return err
		}
		if activeAdmins <= 1 {
			return ErrLastSuperAdmin
		}
	}
	var businessRecords bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM spaces WHERE owner_user_id=$1) OR EXISTS(SELECT 1 FROM wallet_operations WHERE user_id=$1)`, userID).Scan(&businessRecords); err != nil {
		return err
	}
	if businessRecords {
		return ErrUserReferenced
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) RegistrationEnabled(ctx context.Context) (bool, error) {
	var value string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key='registration_enabled'`).Scan(&value); err != nil {
		return false, err
	}
	return value == "true", nil
}

func (s *Store) SetRegistrationEnabled(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE app_settings SET value=$1,updated_at=$2 WHERE key='registration_enabled'`, value, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) AdminSpaces(ctx context.Context) ([]AdminSpaceSummary, AdminPlatformCounts, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
s.id,s.name,u.username,u.display_name,s.newapi_base_url,s.admin_token_last4,s.quota_per_usd,s.created_at,
(SELECT COUNT(*) FROM space_members m WHERE m.space_id=s.id),
(SELECT COUNT(*) FROM space_members m WHERE m.space_id=s.id AND m.newapi_user_id IS NOT NULL),
(SELECT COUNT(*) FROM table_states t WHERE t.space_id=s.id),
(SELECT COUNT(*) FROM wallet_operations w WHERE w.space_id=s.id),
(SELECT COUNT(*) FROM wallet_operations w WHERE w.space_id=s.id AND w.status='failed')
FROM spaces s JOIN users u ON u.id=s.owner_user_id ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, AdminPlatformCounts{}, err
	}
	defer rows.Close()
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

func (s *Store) CreateSpace(ctx context.Context, space Space) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO spaces(
id,name,invite_code,owner_user_id,newapi_base_url,admin_token_enc,admin_token_last4,admin_newapi_user_id,admin_newapi_role,quota_per_usd,created_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, space.ID, space.Name, space.InviteCode, space.OwnerUserID, space.BaseURL, space.AdminTokenEnc, space.AdminTokenLast4, space.AdminNewAPIUserID, space.AdminNewAPIRole, space.QuotaPerUSD, space.CreatedAt)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO space_members(space_id,user_id) VALUES($1,$2)`, space.ID, space.OwnerUserID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SpacesForUser(ctx context.Context, userID int64) ([]Space, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.name,s.invite_code,s.owner_user_id,s.newapi_base_url,s.admin_token_last4,
s.admin_newapi_user_id,s.admin_newapi_role,s.quota_per_usd,s.created_at,
(s.owner_user_id = $1),
(m.newapi_user_id IS NOT NULL)
FROM spaces s JOIN space_members m ON m.space_id=s.id WHERE m.user_id=$1 ORDER BY s.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var spaces []Space
	for rows.Next() {
		var space Space
		if err := rows.Scan(&space.ID, &space.Name, &space.InviteCode, &space.OwnerUserID, &space.BaseURL, &space.AdminTokenLast4,
			&space.AdminNewAPIUserID, &space.AdminNewAPIRole, &space.QuotaPerUSD, &space.CreatedAt, &space.IsOwner, &space.IsBound); err != nil {
			return nil, err
		}
		if !space.IsOwner {
			space.InviteCode = ""
		}
		space.CanManage = space.IsOwner
		spaces = append(spaces, space)
	}
	return spaces, rows.Err()
}

func (s *Store) SpaceForUser(ctx context.Context, spaceID string, userID int64) (Space, error) {
	var space Space
	err := s.db.QueryRowContext(ctx, `SELECT s.id,s.name,s.invite_code,s.owner_user_id,s.newapi_base_url,s.admin_token_enc,s.admin_token_last4,
s.admin_newapi_user_id,s.admin_newapi_role,s.quota_per_usd,s.created_at,
(s.owner_user_id = $1),
(m.newapi_user_id IS NOT NULL)
FROM spaces s JOIN space_members m ON m.space_id=s.id WHERE s.id=$2 AND m.user_id=$1`, userID, spaceID).
		Scan(&space.ID, &space.Name, &space.InviteCode, &space.OwnerUserID, &space.BaseURL, &space.AdminTokenEnc, &space.AdminTokenLast4,
			&space.AdminNewAPIUserID, &space.AdminNewAPIRole, &space.QuotaPerUSD, &space.CreatedAt, &space.IsOwner, &space.IsBound)
	if err == nil && !space.IsOwner {
		space.InviteCode = ""
	}
	space.CanManage = space.IsOwner
	return space, mapNotFound(err)
}

func (s *Store) SpaceByID(ctx context.Context, spaceID string) (Space, error) {
	var space Space
	err := s.db.QueryRowContext(ctx, `SELECT id,name,invite_code,owner_user_id,newapi_base_url,admin_token_enc,admin_token_last4,
admin_newapi_user_id,admin_newapi_role,quota_per_usd,created_at FROM spaces WHERE id=$1`, spaceID).
		Scan(&space.ID, &space.Name, &space.InviteCode, &space.OwnerUserID, &space.BaseURL, &space.AdminTokenEnc, &space.AdminTokenLast4,
			&space.AdminNewAPIUserID, &space.AdminNewAPIRole, &space.QuotaPerUSD, &space.CreatedAt)
	return space, mapNotFound(err)
}

func (s *Store) JoinSpace(ctx context.Context, inviteCode string, userID int64) (Space, error) {
	var spaceID string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM spaces WHERE LOWER(invite_code)=LOWER($1)`, inviteCode).Scan(&spaceID); err != nil {
		return Space{}, mapNotFound(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO space_members(space_id,user_id) VALUES($1,$2) ON CONFLICT(space_id,user_id) DO NOTHING`, spaceID, userID); err != nil {
		return Space{}, err
	}
	return s.SpaceForUser(ctx, spaceID, userID)
}

func (s *Store) UpdateSpaceConnection(ctx context.Context, spaceID string, ownerID int64, baseURL, tokenEnc, last4 string, adminUserID int64, adminRole int, quotaPerUSD int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE spaces SET newapi_base_url=$1,admin_token_enc=$2,admin_token_last4=$3,admin_newapi_user_id=$4,admin_newapi_role=$5,quota_per_usd=$6 WHERE id=$7 AND owner_user_id=$8`,
		baseURL, tokenEnc, last4, adminUserID, adminRole, quotaPerUSD, spaceID, ownerID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Member(ctx context.Context, spaceID string, userID int64) (Member, error) {
	var member Member
	var newUserID sql.NullInt64
	var username, displayName, tokenEnc, last4, verified sql.NullString
	var role sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT m.space_id,m.user_id,m.newapi_user_id,m.newapi_username,m.newapi_display_name,m.newapi_role,m.user_token_enc,m.user_token_last4,m.verified_at,u.display_name
FROM space_members m JOIN users u ON u.id=m.user_id WHERE m.space_id=$1 AND m.user_id=$2`, spaceID, userID).
		Scan(&member.SpaceID, &member.UserID, &newUserID, &username, &displayName, &role, &tokenEnc, &last4, &verified, &member.PokerDisplayName)
	if err != nil {
		return Member{}, mapNotFound(err)
	}
	member.NewAPIUserID = newUserID.Int64
	member.NewAPIUsername = username.String
	member.NewAPIDisplay = displayName.String
	member.NewAPIRole = int(role.Int64)
	member.UserTokenEnc = tokenEnc.String
	member.UserTokenLast4 = last4.String
	member.VerifiedAt = verified.String
	return member, nil
}

func (s *Store) Members(ctx context.Context, spaceID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.space_id,m.user_id,m.newapi_user_id,m.newapi_username,m.newapi_display_name,m.newapi_role,m.user_token_enc,m.user_token_last4,m.verified_at,u.display_name
FROM space_members m JOIN users u ON u.id=m.user_id WHERE m.space_id=$1 ORDER BY u.display_name,u.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []Member
	for rows.Next() {
		var member Member
		var newUserID, role sql.NullInt64
		var username, displayName, tokenEnc, last4, verified sql.NullString
		if err := rows.Scan(&member.SpaceID, &member.UserID, &newUserID, &username, &displayName, &role, &tokenEnc, &last4, &verified, &member.PokerDisplayName); err != nil {
			return nil, err
		}
		member.NewAPIUserID = newUserID.Int64
		member.NewAPIUsername = username.String
		member.NewAPIDisplay = displayName.String
		member.NewAPIRole = int(role.Int64)
		member.UserTokenEnc = tokenEnc.String
		member.UserTokenLast4 = last4.String
		member.VerifiedAt = verified.String
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Store) BindMember(ctx context.Context, member Member) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE space_members SET newapi_user_id=$1,newapi_username=$2,newapi_display_name=$3,newapi_role=$4,user_token_enc=$5,user_token_last4=$6,verified_at=$7 WHERE space_id=$8 AND user_id=$9`,
		member.NewAPIUserID, member.NewAPIUsername, member.NewAPIDisplay, member.NewAPIRole, member.UserTokenEnc, member.UserTokenLast4, now, member.SpaceID, member.UserID)
	return err
}

func (s *Store) CreateWalletOperation(ctx context.Context, operation WalletOperation) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO wallet_operations(id,space_id,table_id,user_id,newapi_user_id,actor_user_id,kind,cents,quota,note,status,error,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		operation.ID, operation.SpaceID, operation.TableID, operation.UserID, operation.NewAPIUserID, operation.ActorUserID, operation.Kind, operation.Cents, operation.Quota, operation.Note, operation.Status, "", now, now)
	return err
}

func (s *Store) UpdateWalletOperation(ctx context.Context, operationID, status, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE wallet_operations SET status=$1,error=$2,updated_at=$3 WHERE id=$4`, status, errorMessage, time.Now().UTC().Format(time.RFC3339Nano), operationID)
	return err
}

func (s *Store) WalletOperations(ctx context.Context, spaceID string, userID int64) ([]WalletOperation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,space_id,table_id,user_id,newapi_user_id,actor_user_id,kind,cents,quota,note,status,error,created_at,updated_at FROM wallet_operations WHERE space_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT 50`, spaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var operations []WalletOperation
	for rows.Next() {
		var operation WalletOperation
		if err := rows.Scan(&operation.ID, &operation.SpaceID, &operation.TableID, &operation.UserID, &operation.NewAPIUserID, &operation.ActorUserID, &operation.Kind, &operation.Cents, &operation.Quota, &operation.Note, &operation.Status, &operation.Error, &operation.CreatedAt, &operation.UpdatedAt); err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func (s *Store) ChannelLeaderboard(ctx context.Context, spaceID string) ([]ChannelLeaderboardEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id,u.display_name,
COALESCE(SUM(CASE
  WHEN w.status='completed' AND w.kind='cash_out' THEN w.cents
  WHEN w.status='completed' AND w.kind='buy_in' THEN -w.cents
  ELSE 0
END),0)::BIGINT AS net_cents,
COUNT(*) FILTER (WHERE w.status='completed' AND w.kind='buy_in')::INTEGER AS sessions
FROM space_members m
JOIN users u ON u.id=m.user_id
LEFT JOIN wallet_operations w ON w.space_id=m.space_id AND w.user_id=m.user_id
WHERE m.space_id=$1 AND u.ranking_hidden=FALSE
GROUP BY u.id,u.display_name
ORDER BY net_cents DESC,sessions DESC,LOWER(u.display_name),u.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]ChannelLeaderboardEntry, 0)
	for rows.Next() {
		var entry ChannelLeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.DisplayName, &entry.NetCents, &entry.Sessions); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) SaveTableState(ctx context.Context, spaceID, tableID string, data []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO table_states(space_id,table_id,state_json,updated_at) VALUES($1,$2,$3,$4)
ON CONFLICT(space_id,table_id) DO UPDATE SET state_json=excluded.state_json,updated_at=excluded.updated_at`, spaceID, tableID, data, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) LoadTableState(ctx context.Context, spaceID, tableID string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT state_json FROM table_states WHERE space_id=$1 AND table_id=$2`, spaceID, tableID).Scan(&data)
	return data, mapNotFound(err)
}

func (s *Store) DeleteTableState(ctx context.Context, spaceID, tableID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM table_states WHERE space_id=$1 AND table_id=$2`, spaceID, tableID)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TableStateIDs(ctx context.Context, spaceID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT table_id FROM table_states WHERE space_id=$1 ORDER BY updated_at DESC`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func mapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func IsUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}

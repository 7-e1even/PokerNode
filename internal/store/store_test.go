package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestOnlySuperAdminIsSystemRole(t *testing.T) {
	ctx := context.Background()
	databaseURL := newTestDatabaseURL(t)
	database, err := Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	roles, err := database.Roles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 4 {
		t.Fatalf("expected four initial roles, got %#v", roles)
	}
	for _, role := range roles {
		if role.System != (role.Key == "super_admin") {
			t.Fatalf("unexpected system flag for %s: %v", role.Key, role.System)
		}
	}
	updated, err := database.UpdateRole(ctx, "operator", "自定义运营", "可修改的默认角色", []string{"admin:view"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.System || updated.Name != "自定义运营" {
		t.Fatalf("default role was not editable: %#v", updated)
	}
	if _, err := database.db.ExecContext(ctx, `UPDATE roles SET system=TRUE`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key='default_roles_seeded'`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	roles, err = database.Roles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if role.System != (role.Key == "super_admin") {
			t.Fatalf("legacy system flag was not normalized for %s: %v", role.Key, role.System)
		}
	}
	if err := database.DeleteRole(ctx, "channel_manager"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.RoleByKey(ctx, "channel_manager"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted default role was recreated after restart: %v", err)
	}
	operator, err := database.RoleByKey(ctx, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if operator.Name != "自定义运营" || operator.System {
		t.Fatalf("customized default role did not persist: %#v", operator)
	}
	if _, err := database.UpdateRole(ctx, "super_admin", "Changed", "", nil); !errors.Is(err, ErrSystemRole) {
		t.Fatalf("updating the super administrator role returned %v", err)
	}
}

func TestUserDeletionGuards(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t)

	admin, err := database.CreateUser(ctx, "admin", "Admin", "hash", "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateUser(ctx, admin.ID, admin.Username, admin.DisplayName, admin.PasswordHash, "player", "active"); !errors.Is(err, ErrLastSuperAdmin) {
		t.Fatalf("demoting the last active administrator returned %v", err)
	}
	if err := database.DeleteUser(ctx, admin.ID); !errors.Is(err, ErrLastSuperAdmin) {
		t.Fatalf("deleting the last active administrator returned %v", err)
	}

	owner, err := database.CreateUser(ctx, "owner", "Owner", "hash", "player")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSpace(ctx, Space{
		ID: "space-1", Name: "Space", InviteCode: "ABC123", OwnerUserID: owner.ID,
		BaseURL: "http://example.test", AdminTokenEnc: "encrypted", AdminTokenLast4: "last",
		AdminNewAPIUserID: 1, AdminNewAPIRole: 100, QuotaPerUSD: 500000, CreatedAt: "2026-08-18T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteUser(ctx, owner.ID); !errors.Is(err, ErrUserReferenced) {
		t.Fatalf("deleting a channel owner returned %v", err)
	}

	player, err := database.CreateUser(ctx, "player", "Player", "hash", "player")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteUser(ctx, player.ID); err != nil {
		t.Fatalf("deleting an unreferenced player: %v", err)
	}
	if _, err := database.UserByID(ctx, player.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted player lookup returned %v", err)
	}
}

func TestRolesAndMultiChannelManagementScope(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t)

	role, err := database.CreateRole(ctx, Role{
		Key: "regional_manager", Name: "区域频道管理员", Description: "管理分配的频道",
		Permissions: []string{"admin:view", "channels:manage", "balances:manage"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if role.System || len(role.Permissions) != 3 {
		t.Fatalf("unexpected custom role: %#v", role)
	}

	owner, err := database.CreateUser(ctx, "scope_owner", "Scope Owner", "hash", "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := database.CreateUser(ctx, "scope_manager", "Scope Manager", "hash", role.Key)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"scope-a", "scope-b", "scope-c"} {
		if err := database.CreateSpace(ctx, Space{
			ID: id, Name: id, InviteCode: "INV-" + id, OwnerUserID: owner.ID,
			BaseURL: "http://example.test", AdminTokenEnc: "encrypted", AdminTokenLast4: "last",
			AdminNewAPIUserID: 1, AdminNewAPIRole: 100, QuotaPerUSD: 500000, CreatedAt: "2026-08-19T00:00:00Z",
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, inviteCode := range []string{"INV-scope-a", "INV-scope-b"} {
		if _, err := database.JoinSpace(ctx, inviteCode, manager.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.SetManagedSpaces(ctx, manager.ID, []string{"scope-c"}); !errors.Is(err, ErrSpaceMembershipRequired) {
		t.Fatalf("assigning an unjoined channel returned %v", err)
	}
	if _, err := database.db.ExecContext(ctx, `INSERT INTO space_managers(space_id,user_id,created_at) VALUES($1,$2,$3)`, "scope-c", manager.ID, "2026-08-19T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if assigned, err := database.IsSpaceManager(ctx, "scope-c", manager.ID); err != nil || assigned {
		t.Fatalf("stale assignment without membership should not grant access: assigned=%v err=%v", assigned, err)
	}
	if err := database.SetManagedSpaces(ctx, manager.ID, []string{"scope-a", "scope-b"}); err != nil {
		t.Fatal(err)
	}
	joined, err := database.JoinedSpaceIDs(ctx, manager.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(joined) != 2 || joined[0] != "scope-a" || joined[1] != "scope-b" {
		t.Fatalf("expected two joined channels, got %#v", joined)
	}
	assigned, err := database.ManagedSpaceIDs(ctx, manager.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assigned) != 2 || assigned[0] != "scope-a" || assigned[1] != "scope-b" {
		t.Fatalf("expected two assigned channels, got %#v", assigned)
	}
	spaces, err := database.SpacesForActor(ctx, manager.ID, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 2 || !spaces[0].CanManage || !spaces[1].CanManage {
		t.Fatalf("expected two manageable channels, got %#v", spaces)
	}
	if _, err := database.SpaceForActor(ctx, "scope-c", manager.ID, true, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unassigned channel lookup returned %v", err)
	}
	allSpaces, err := database.SpacesForActor(ctx, owner.ID, false, true)
	if err != nil || len(allSpaces) != 3 {
		t.Fatalf("super administrator scope = %d, %v", len(allSpaces), err)
	}
	if err := database.DeleteRole(ctx, role.Key); !errors.Is(err, ErrRoleReferenced) {
		t.Fatalf("deleting an assigned role returned %v", err)
	}
	if _, err := database.UpdateRole(ctx, "super_admin", "Changed", "", nil); !errors.Is(err, ErrSystemRole) {
		t.Fatalf("updating a system role returned %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := Open(newTestDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func newTestDatabaseURL(t *testing.T) string {
	t.Helper()
	baseURL := os.Getenv("POKERNODE_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("POKERNODE_TEST_DATABASE_URL is required for PostgreSQL store tests")
	}
	admin, err := sql.Open("pgx", baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.PingContext(context.Background()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "pokernode_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(context.Background(), `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		_ = admin.Close()
	})
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

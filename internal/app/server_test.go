package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"pokernode/internal/auth"
	"pokernode/internal/secure"
	"pokernode/internal/store"
)

type fakeNewAPI struct {
	mu     sync.Mutex
	users  map[string]map[string]any
	quotas map[int64]int64
}

type autoProvisionedUser struct {
	ID          int64
	Username    string
	Password    string
	DisplayName string
}

func TestFullSpaceAndTableFlow(t *testing.T) {
	upstream := &fakeNewAPI{
		users: map[string]map[string]any{
			"admin-token":   {"id": int64(99), "username": "root", "display_name": "Root", "role": 100, "status": 1},
			"alice-token":   {"id": int64(1), "username": "alice-newapi", "display_name": "Alice API", "role": 1, "status": 1},
			"bob-token":     {"id": int64(2), "username": "bob-newapi", "display_name": "Bob API", "role": 1, "status": 1},
			"bob-alt-token": {"id": int64(3), "username": "bob-existing", "display_name": "Bob Existing", "role": 1, "status": 1},
		},
		quotas: map[int64]int64{1: 100_000_000, 2: 100_000_000, 3: 100_000_000, 99: 100_000_000},
	}
	newAPIServer := httptest.NewServer(http.HandlerFunc(upstream.serveHTTP))
	defer newAPIServer.Close()

	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions("test-session-secret-that-is-long-enough", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	appServer := httptest.NewServer(NewServer(database, cipher, sessions, logger).Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()
	requestJSON(t, newTestClient(t), http.MethodGet, appServer.URL+"/healthz", nil, http.StatusOK, nil)
	requestJSON(t, newTestClient(t), http.MethodGet, appServer.URL+"/readyz", nil, http.StatusOK, nil)

	alice := newTestClient(t)
	bob := newTestClient(t)
	requestJSON(t, alice, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "alice", "display_name": "Alice", "password": "password-1"}, http.StatusCreated, nil)
	var createResult struct {
		Space struct {
			ID         string `json:"id"`
			InviteCode string `json:"invite_code"`
		} `json:"space"`
	}
	requestJSON(t, alice, http.MethodPost, appServer.URL+"/api/spaces", map[string]any{
		"name": "Friday", "newapi_base_url": newAPIServer.URL + "/profile", "admin_token": "admin-token", "quota_per_usd": 500_000,
	}, http.StatusCreated, &createResult)
	if createResult.Space.ID == "" || createResult.Space.InviteCode == "" {
		t.Fatal("space id and invite code should be returned")
	}
	spacePath := appServer.URL + "/api/spaces/" + createResult.Space.ID
	var tablesResult struct {
		Tables []struct {
			ID      string `json:"id"`
			Players []struct {
				Name string `json:"name"`
				Seat int    `json:"seat"`
			} `json:"players"`
		} `json:"tables"`
	}
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	if len(tablesResult.Tables) != 1 || tablesResult.Tables[0].ID != mainTableID {
		t.Fatalf("expected the default table, got %#v", tablesResult.Tables)
	}
	var createTableResult struct {
		Table struct {
			ID string `json:"id"`
		} `json:"table"`
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/tables", map[string]any{
		"name": "High Stakes", "small_blind_cents": 100, "big_blind_cents": 200,
	}, http.StatusCreated, &createTableResult)
	if createTableResult.Table.ID == "" || createTableResult.Table.ID == mainTableID {
		t.Fatal("expected a second table id")
	}
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	if len(tablesResult.Tables) != 2 {
		t.Fatalf("expected two tables, got %d", len(tablesResult.Tables))
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/bind", map[string]string{"token": "alice-token"}, http.StatusOK, nil)

	requestJSON(t, bob, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "bob", "display_name": "Bob", "password": "password-2"}, http.StatusCreated, nil)
	requestJSON(t, bob, http.MethodPost, appServer.URL+"/api/spaces/join", map[string]string{"invite_code": createResult.Space.InviteCode}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/bind", map[string]string{"token": "bob-token"}, http.StatusOK, nil)
	var accountBindings struct {
		Bindings []accountBindingView `json:"bindings"`
	}
	requestJSON(t, bob, http.MethodGet, appServer.URL+"/api/account-bindings", nil, http.StatusOK, &accountBindings)
	if len(accountBindings.Bindings) != 1 || accountBindings.Bindings[0].Membership.NewAPIUsername != "bob-newapi" {
		t.Fatalf("expected Bob's channel binding, got %#v", accountBindings.Bindings)
	}
	requestJSON(t, bob, http.MethodGet, spacePath+"/managed-balances", nil, http.StatusForbidden, nil)
	var managed struct {
		Members []struct {
			UserID  int64            `json:"user_id"`
			Balance map[string]int64 `json:"balance"`
		} `json:"members"`
	}
	requestJSON(t, alice, http.MethodGet, spacePath+"/managed-balances", nil, http.StatusOK, &managed)
	if len(managed.Members) != 2 {
		t.Fatalf("expected both channel members in balance management, got %d", len(managed.Members))
	}
	var adjustment struct {
		Member struct {
			Balance map[string]int64 `json:"balance"`
		} `json:"member"`
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/managed-balances/2/adjust", map[string]any{
		"direction": "add", "amount_cents": 100, "reason": "Test credit",
	}, http.StatusOK, &adjustment)
	if adjustment.Member.Balance["cents"] != 20_100 {
		t.Fatalf("expected adjusted balance of $201, got %#v", adjustment.Member.Balance)
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/managed-balances/2/adjust", map[string]any{
		"direction": "subtract", "amount_cents": 100, "reason": "Test reversal",
	}, http.StatusOK, nil)
	var operations struct {
		Operations []store.WalletOperation `json:"operations"`
	}
	requestJSON(t, bob, http.MethodGet, spacePath+"/operations", nil, http.StatusOK, &operations)
	if len(operations.Operations) != 2 || operations.Operations[0].ActorUserID != 1 || operations.Operations[0].Note != "Test reversal" {
		t.Fatalf("manual balance audit was incomplete: %#v", operations.Operations)
	}

	var bobSpace struct {
		Space store.Space `json:"space"`
	}
	requestJSON(t, bob, http.MethodPost, appServer.URL+"/api/spaces", map[string]any{
		"name": "Bob's room", "newapi_base_url": newAPIServer.URL, "admin_token": "admin-token", "quota_per_usd": 500_000,
	}, http.StatusCreated, &bobSpace)
	requestJSON(t, bob, http.MethodPost, appServer.URL+"/api/spaces/"+bobSpace.Space.ID+"/bind", map[string]string{"token": "bob-token"}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodGet, appServer.URL+"/api/spaces/"+bobSpace.Space.ID+"/managed-balances", nil, http.StatusOK, nil)
	requestJSON(t, alice, http.MethodGet, appServer.URL+"/api/spaces/"+bobSpace.Space.ID+"/managed-balances", nil, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/tables", map[string]any{
		"name": "Unauthorized", "small_blind_cents": 50, "big_blind_cents": 100,
	}, http.StatusForbidden, nil)
	requestJSON(t, bob, http.MethodGet, spacePath+"/tables/"+createTableResult.Table.ID, nil, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodDelete, spacePath+"/tables/"+createTableResult.Table.ID, nil, http.StatusForbidden, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/tables/"+createTableResult.Table.ID+"/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/bind", map[string]string{"token": "bob-alt-token"}, http.StatusConflict, nil)
	requestJSON(t, alice, http.MethodDelete, spacePath+"/tables/"+createTableResult.Table.ID, nil, http.StatusConflict, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/tables/"+createTableResult.Table.ID+"/leave", map[string]any{}, http.StatusOK, nil)
	var rebound struct {
		Membership store.Member `json:"membership"`
	}
	requestJSON(t, bob, http.MethodPost, spacePath+"/bind", map[string]string{"token": "bob-alt-token"}, http.StatusOK, &rebound)
	if rebound.Membership.NewAPIUserID != 3 || rebound.Membership.NewAPIUsername != "bob-existing" {
		t.Fatalf("expected existing New API account to be bound, got %#v", rebound.Membership)
	}
	requestJSON(t, bob, http.MethodPost, spacePath+"/bind", map[string]string{"token": "bob-token"}, http.StatusOK, nil)
	requestJSON(t, alice, http.MethodDelete, spacePath+"/tables/"+createTableResult.Table.ID, nil, http.StatusNoContent, nil)
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables/"+createTableResult.Table.ID, nil, http.StatusNotFound, nil)
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	if len(tablesResult.Tables) != 1 || tablesResult.Tables[0].ID != mainTableID {
		t.Fatalf("expected only the default table after deletion, got %#v", tablesResult.Tables)
	}

	requestJSON(t, alice, http.MethodPost, spacePath+"/table/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	var mainTablePlayers []struct {
		Name string `json:"name"`
		Seat int    `json:"seat"`
	}
	for _, table := range tablesResult.Tables {
		if table.ID == mainTableID {
			mainTablePlayers = table.Players
		}
	}
	if len(mainTablePlayers) != 2 {
		t.Fatalf("expected table summary to expose two occupied seats, got %d", len(mainTablePlayers))
	}
	playerNames := map[string]bool{}
	for _, player := range mainTablePlayers {
		playerNames[player.Name] = true
	}
	if !playerNames["Alice"] || !playerNames["Bob"] || mainTablePlayers[0].Seat == mainTablePlayers[1].Seat {
		t.Fatalf("expected real players in distinct seats, got %#v", mainTablePlayers)
	}
	var started struct {
		Table struct {
			ViewerSeat int `json:"viewer_seat"`
			ActingSeat int `json:"acting_seat"`
		} `json:"table"`
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/start", map[string]any{}, http.StatusOK, &started)
	if started.Table.ViewerSeat != started.Table.ActingSeat {
		t.Fatalf("expected Alice to act first heads-up, viewer=%d acting=%d", started.Table.ViewerSeat, started.Table.ActingSeat)
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/action", map[string]any{"action": "fold", "amount_cents": 0}, http.StatusOK, nil)
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/leave", map[string]any{}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/leave", map[string]any{}, http.StatusOK, nil)

	upstream.mu.Lock()
	aliceQuota, bobQuota := upstream.quotas[1], upstream.quotas[2]
	upstream.mu.Unlock()
	if aliceQuota != 99_750_000 || bobQuota != 100_250_000 {
		t.Fatalf("unexpected final New API quotas Alice=%d Bob=%d", aliceQuota, bobQuota)
	}
	if aliceQuota+bobQuota != 200_000_000 {
		t.Fatal("player balances should be conserved")
	}
	var leaderboard struct {
		Entries []store.ChannelLeaderboardEntry `json:"leaderboard"`
	}
	requestJSON(t, bob, http.MethodGet, spacePath+"/leaderboard", nil, http.StatusOK, &leaderboard)
	if len(leaderboard.Entries) != 2 || leaderboard.Entries[0].DisplayName != "Bob" || leaderboard.Entries[0].NetCents != 50 || leaderboard.Entries[0].Sessions != 2 {
		t.Fatalf("unexpected channel leaderboard: %#v", leaderboard.Entries)
	}
	if leaderboard.Entries[1].DisplayName != "Alice" || leaderboard.Entries[1].NetCents != -50 || leaderboard.Entries[1].Sessions != 1 {
		t.Fatalf("unexpected second leaderboard entry: %#v", leaderboard.Entries[1])
	}
	requestJSON(t, bob, http.MethodPut, appServer.URL+"/api/admin/rankings/1", map[string]bool{"hidden": true}, http.StatusForbidden, nil)
	requestJSON(t, alice, http.MethodPut, appServer.URL+"/api/admin/rankings/2", map[string]bool{"hidden": true}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodGet, spacePath+"/leaderboard", nil, http.StatusOK, &leaderboard)
	if len(leaderboard.Entries) != 1 || leaderboard.Entries[0].DisplayName != "Alice" {
		t.Fatalf("hidden player must not appear in channel leaderboard: %#v", leaderboard.Entries)
	}
	requestJSON(t, alice, http.MethodPut, appServer.URL+"/api/admin/rankings/2", map[string]bool{"hidden": false}, http.StatusOK, nil)
	requestJSON(t, alice, http.MethodDelete, spacePath+"/tables/"+mainTableID, nil, http.StatusNoContent, nil)
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	if len(tablesResult.Tables) != 0 {
		t.Fatalf("expected no tables after deleting the default table, got %#v", tablesResult.Tables)
	}
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	if len(tablesResult.Tables) != 0 {
		t.Fatalf("deleted default table must not be recreated, got %#v", tablesResult.Tables)
	}
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables/"+mainTableID, nil, http.StatusNotFound, nil)
}

func TestCreateAndJoinSpaceAutomaticallyProvisionMembers(t *testing.T) {
	var upstreamMu sync.Mutex
	users := make(map[string]autoProvisionedUser)
	loginTokens := make(map[string]string)
	personalTokens := make(map[string]string)
	var nextUserID int64
	var createCalls int
	var rejectCreates bool
	newAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		upstreamMu.Lock()
		defer upstreamMu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/self" && token == "admin-token":
			writeFake(w, http.StatusOK, true, map[string]any{
				"id": int64(99), "username": "root", "display_name": "Root", "role": 100, "status": 1, "quota": int64(100_000_000),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/" && token == "admin-token":
			var input struct {
				Username    string `json:"username"`
				Password    string `json:"password"`
				DisplayName string `json:"display_name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			if len(input.Password) < 8 || len(input.Password) > 20 {
				writeFake(w, http.StatusOK, false, nil)
				return
			}
			if rejectCreates {
				writeFake(w, http.StatusOK, false, nil)
				return
			}
			if _, exists := users[input.Username]; exists {
				writeFake(w, http.StatusOK, false, nil)
				return
			}
			nextUserID++
			createCalls++
			users[input.Username] = autoProvisionedUser{
				ID: nextUserID, Username: input.Username, Password: input.Password, DisplayName: input.DisplayName,
			}
			writeFake(w, http.StatusOK, true, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/login":
			var input struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			newUser, exists := users[input.Username]
			if !exists || newUser.Password != input.Password {
				writeFake(w, http.StatusUnauthorized, false, nil)
				return
			}
			loginToken := "login-" + input.Username
			loginTokens[loginToken] = input.Username
			writeFake(w, http.StatusOK, true, map[string]any{
				"access_token": loginToken,
				"user":         map[string]any{"id": newUser.ID, "username": newUser.Username, "display_name": newUser.DisplayName, "role": 1, "status": 1},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/token":
			username, exists := loginTokens[token]
			if !exists {
				writeFake(w, http.StatusUnauthorized, false, nil)
				return
			}
			personalToken := "pat-" + username
			personalTokens[personalToken] = username
			writeFake(w, http.StatusOK, true, personalToken)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/self":
			username, exists := personalTokens[token]
			newUser, userExists := users[username]
			if !exists || !userExists {
				writeFake(w, http.StatusUnauthorized, false, nil)
				return
			}
			writeFake(w, http.StatusOK, true, map[string]any{
				"id": newUser.ID, "username": newUser.Username, "display_name": newUser.DisplayName,
				"role": 1, "status": 1, "quota": int64(10_000_000),
			})
		default:
			writeFake(w, http.StatusNotFound, false, nil)
		}
	}))
	defer newAPIServer.Close()

	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions("test-session-secret-that-is-long-enough", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	appServer := httptest.NewServer(NewServer(database, cipher, sessions, logger).Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()

	owner := newTestClient(t)
	member := newTestClient(t)
	requestJSON(t, owner, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "owner", "display_name": "Owner", "password": "password-1",
	}, http.StatusCreated, nil)
	var created struct {
		Space store.Space `json:"space"`
	}
	requestJSON(t, owner, http.MethodPost, appServer.URL+"/api/spaces", map[string]any{
		"name": "Automatic", "newapi_base_url": newAPIServer.URL, "admin_token": "admin-token", "quota_per_usd": 500_000,
	}, http.StatusCreated, &created)
	if !created.Space.IsBound {
		t.Fatal("channel owner should be automatically bound")
	}

	requestJSON(t, member, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "member", "display_name": "Member", "password": "password-2",
	}, http.StatusCreated, nil)
	var joined struct {
		Space store.Space `json:"space"`
	}
	requestJSON(t, member, http.MethodPost, appServer.URL+"/api/spaces/join", map[string]string{
		"invite_code": created.Space.InviteCode,
	}, http.StatusOK, &joined)
	if !joined.Space.IsBound {
		t.Fatal("joining member should be automatically bound")
	}
	var detail struct {
		Membership store.Member `json:"membership"`
	}
	requestJSON(t, member, http.MethodGet, appServer.URL+"/api/spaces/"+created.Space.ID, nil, http.StatusOK, &detail)
	if detail.Membership.NewAPIUserID == 0 || !strings.HasPrefix(detail.Membership.NewAPIUsername, "pn_") || detail.Membership.UserTokenLast4 == "" {
		t.Fatalf("automatic membership was incomplete: %#v", detail.Membership)
	}

	requestJSON(t, member, http.MethodPost, appServer.URL+"/api/spaces/join", map[string]string{
		"invite_code": created.Space.InviteCode,
	}, http.StatusOK, &joined)
	upstreamMu.Lock()
	gotCreateCalls := createCalls
	upstreamMu.Unlock()
	if gotCreateCalls != 2 {
		t.Fatalf("expected one New API user per PokerNode member, got %d creates", gotCreateCalls)
	}

	retryingMember := newTestClient(t)
	requestJSON(t, retryingMember, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "retrying_member", "display_name": "Retrying Member", "password": "password-3",
	}, http.StatusCreated, nil)
	upstreamMu.Lock()
	rejectCreates = true
	upstreamMu.Unlock()
	var failedJoin struct {
		Space   store.Space `json:"space"`
		Warning string      `json:"warning"`
	}
	requestJSON(t, retryingMember, http.MethodPost, appServer.URL+"/api/spaces/join", map[string]string{
		"invite_code": created.Space.InviteCode,
	}, http.StatusOK, &failedJoin)
	if failedJoin.Warning == "" || failedJoin.Space.IsBound {
		t.Fatalf("expected the simulated first provisioning attempt to fail: %#v", failedJoin)
	}
	upstreamMu.Lock()
	rejectCreates = false
	upstreamMu.Unlock()
	var recovered struct {
		Space      store.Space  `json:"space"`
		Membership store.Member `json:"membership"`
	}
	requestJSON(t, retryingMember, http.MethodGet, appServer.URL+"/api/spaces/"+created.Space.ID, nil, http.StatusOK, &recovered)
	if !recovered.Space.IsBound || recovered.Membership.NewAPIUserID == 0 {
		t.Fatalf("expected channel entry to recover automatic provisioning: %#v", recovered)
	}
}

func TestRegistrationToggleAndRoleBoundaries(t *testing.T) {
	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions("test-session-secret-that-is-long-enough", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	appServer := httptest.NewServer(NewServer(database, cipher, sessions, logger).Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()

	admin := newTestClient(t)
	operator := newTestClient(t)
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "admin", "display_name": "Admin", "password": "password-1"}, http.StatusCreated, nil)
	var operatorResult struct {
		User store.User `json:"user"`
	}
	requestJSON(t, operator, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "operator", "display_name": "Operator", "password": "password-2"}, http.StatusCreated, &operatorResult)

	var emptyOverview struct {
		Users             json.RawMessage `json:"users"`
		Spaces            json.RawMessage `json:"spaces"`
		Permissions       json.RawMessage `json:"permissions"`
		Roles             json.RawMessage `json:"roles"`
		PermissionCatalog json.RawMessage `json:"permission_catalog"`
	}
	requestJSON(t, admin, http.MethodGet, appServer.URL+"/api/admin/overview", nil, http.StatusOK, &emptyOverview)
	for field, value := range map[string]json.RawMessage{
		"users": emptyOverview.Users, "spaces": emptyOverview.Spaces, "permissions": emptyOverview.Permissions,
		"roles": emptyOverview.Roles, "permission_catalog": emptyOverview.PermissionCatalog,
	} {
		if len(value) == 0 || value[0] != '[' {
			t.Fatalf("admin overview %s must be a JSON array, got %s", field, value)
		}
	}
	adminLogin := newTestClient(t)
	requestJSON(t, adminLogin, http.MethodPost, appServer.URL+"/api/admin/auth/login", map[string]any{"username": "admin", "password": "password-1"}, http.StatusOK, nil)
	requestJSON(t, adminLogin, http.MethodGet, appServer.URL+"/api/admin/overview", nil, http.StatusOK, nil)
	requestJSON(t, operator, http.MethodGet, appServer.URL+"/api/admin/overview", nil, http.StatusForbidden, nil)
	requestJSON(t, admin, http.MethodPatch, appServer.URL+"/api/admin/users/"+strconv.FormatInt(operatorResult.User.ID, 10), map[string]string{"role": "operator", "status": "active"}, http.StatusOK, nil)
	requestJSON(t, operator, http.MethodGet, appServer.URL+"/api/admin/overview", nil, http.StatusOK, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/admin/auth/login", map[string]any{"username": "operator", "password": "password-2"}, http.StatusOK, nil)
	var customRole struct {
		Role store.Role `json:"role"`
	}
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/roles", map[string]any{
		"key": "auditor", "name": "审计员", "description": "只读查看", "permissions": []string{"admin:view", "users:read"},
	}, http.StatusCreated, &customRole)
	if customRole.Role.Key != "auditor" || customRole.Role.System {
		t.Fatalf("unexpected custom role: %#v", customRole.Role)
	}
	requestJSON(t, operator, http.MethodGet, appServer.URL+"/api/admin/roles", nil, http.StatusForbidden, nil)
	var auditorResult struct {
		User store.User `json:"user"`
	}
	auditor := newTestClient(t)
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{"username": "auditor", "display_name": "Auditor", "password": "password-5", "role": "auditor"}, http.StatusCreated, &auditorResult)
	requestJSON(t, auditor, http.MethodPost, appServer.URL+"/api/admin/auth/login", map[string]any{"username": "auditor", "password": "password-5"}, http.StatusOK, nil)
	requestJSON(t, auditor, http.MethodGet, appServer.URL+"/api/admin/overview", nil, http.StatusOK, nil)
	requestJSON(t, admin, http.MethodDelete, appServer.URL+"/api/admin/roles/auditor", nil, http.StatusConflict, nil)
	requestJSON(t, admin, http.MethodPatch, appServer.URL+"/api/admin/users/"+strconv.FormatInt(auditorResult.User.ID, 10), map[string]string{"role": "player", "status": "active"}, http.StatusOK, nil)
	requestJSON(t, admin, http.MethodDelete, appServer.URL+"/api/admin/roles/auditor", nil, http.StatusNoContent, nil)

	var playerResult struct {
		User store.User `json:"user"`
	}
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{"username": "player", "display_name": "Player", "password": "password-3", "role": "player"}, http.StatusCreated, &playerResult)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/admin/auth/login", map[string]any{"username": "player", "password": "password-3"}, http.StatusForbidden, nil)
	requestJSON(t, operator, http.MethodPatch, appServer.URL+"/api/admin/users/"+strconv.FormatInt(playerResult.User.ID, 10), map[string]string{"role": "super_admin", "status": "active"}, http.StatusForbidden, nil)
	requestJSON(t, operator, http.MethodPatch, appServer.URL+"/api/admin/users/"+strconv.FormatInt(playerResult.User.ID, 10), map[string]string{"role": "player", "status": "disabled"}, http.StatusOK, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{"username": "player", "password": "password-3"}, http.StatusForbidden, nil)
	var updatedPlayer struct {
		User store.User `json:"user"`
	}
	requestJSON(t, admin, http.MethodPatch, appServer.URL+"/api/admin/users/"+strconv.FormatInt(playerResult.User.ID, 10), map[string]string{
		"username": "renamed_player", "display_name": "Renamed Player", "password": "new-password-3", "role": "player", "status": "active",
	}, http.StatusOK, &updatedPlayer)
	if updatedPlayer.User.Username != "renamed_player" || updatedPlayer.User.DisplayName != "Renamed Player" {
		t.Fatalf("account details were not updated: %#v", updatedPlayer.User)
	}
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{"username": "renamed_player", "password": "password-3"}, http.StatusUnauthorized, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{"username": "renamed_player", "password": "new-password-3"}, http.StatusOK, nil)
	requestJSON(t, operator, http.MethodDelete, appServer.URL+"/api/admin/users/"+strconv.FormatInt(playerResult.User.ID, 10), nil, http.StatusNoContent, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{"username": "renamed_player", "password": "new-password-3"}, http.StatusUnauthorized, nil)
	requestJSON(t, operator, http.MethodDelete, appServer.URL+"/api/admin/users/1", nil, http.StatusForbidden, nil)
	requestJSON(t, admin, http.MethodDelete, appServer.URL+"/api/admin/users/1", nil, http.StatusBadRequest, nil)

	requestJSON(t, admin, http.MethodPut, appServer.URL+"/api/admin/settings/registration", map[string]bool{"enabled": false}, http.StatusOK, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "blocked", "display_name": "Blocked", "password": "password-4"}, http.StatusForbidden, nil)
}

func TestChannelManagerCanManageMultipleAssignedChannels(t *testing.T) {
	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{11}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions("test-session-secret-that-is-long-enough", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	appServer := httptest.NewServer(NewServer(database, cipher, sessions, logger).Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()

	admin := newTestClient(t)
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "scopeadmin", "display_name": "Scope Admin", "password": "password-1"}, http.StatusCreated, nil)
	for index, id := range []string{"managed-a", "managed-b", "unassigned-c"} {
		if err := database.CreateSpace(context.Background(), store.Space{
			ID: id, Name: id, InviteCode: "INV-" + id, OwnerUserID: 1,
			BaseURL: "http://example.test", AdminTokenEnc: "unused", AdminTokenLast4: "last",
			AdminNewAPIUserID: 99, AdminNewAPIRole: 100, QuotaPerUSD: 500000,
			CreatedAt: time.Date(2026, 8, 19, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var createdManager struct {
		User store.User `json:"user"`
	}
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{
		"username": "channelmanager", "display_name": "Channel Manager", "password": "password-2",
		"role": "channel_manager", "managed_space_ids": []string{"managed-a"},
	}, http.StatusBadRequest, nil)
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{
		"username": "channelmanager", "display_name": "Channel Manager", "password": "password-2",
		"role": "channel_manager", "managed_space_ids": []string{},
	}, http.StatusCreated, &createdManager)
	for _, inviteCode := range []string{"INV-managed-a", "INV-managed-b"} {
		if _, err := database.JoinSpace(context.Background(), inviteCode, createdManager.User.ID); err != nil {
			t.Fatal(err)
		}
	}
	requestJSON(t, admin, http.MethodPatch, appServer.URL+"/api/admin/users/"+strconv.FormatInt(createdManager.User.ID, 10), map[string]any{
		"role": "channel_manager", "status": "active", "managed_space_ids": []string{"managed-a", "unassigned-c"},
	}, http.StatusBadRequest, nil)
	var updatedManager struct {
		User store.User `json:"user"`
	}
	requestJSON(t, admin, http.MethodPatch, appServer.URL+"/api/admin/users/"+strconv.FormatInt(createdManager.User.ID, 10), map[string]any{
		"role": "channel_manager", "status": "active", "managed_space_ids": []string{"managed-a", "managed-b"},
	}, http.StatusOK, &updatedManager)
	if len(updatedManager.User.JoinedSpaceIDs) != 2 || len(updatedManager.User.ManagedSpaceIDs) != 2 {
		t.Fatalf("joined and managed channel scopes were not returned: %#v", updatedManager.User)
	}

	manager := newTestClient(t)
	requestJSON(t, manager, http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{"username": "channelmanager", "password": "password-2"}, http.StatusOK, nil)
	var managerSpaces struct {
		Spaces []store.Space `json:"spaces"`
	}
	requestJSON(t, manager, http.MethodGet, appServer.URL+"/api/spaces", nil, http.StatusOK, &managerSpaces)
	if len(managerSpaces.Spaces) != 2 || !managerSpaces.Spaces[0].CanManage || !managerSpaces.Spaces[1].CanManage {
		t.Fatalf("channel manager scope was not applied: %#v", managerSpaces.Spaces)
	}
	for _, id := range []string{"managed-a", "managed-b"} {
		requestJSON(t, manager, http.MethodPost, appServer.URL+"/api/spaces/"+id+"/tables", map[string]any{
			"name": "Managed table", "small_blind_cents": 50, "big_blind_cents": 100,
		}, http.StatusCreated, nil)
	}
	requestJSON(t, manager, http.MethodPost, appServer.URL+"/api/spaces/unassigned-c/tables", map[string]any{
		"name": "Out of scope", "small_blind_cents": 50, "big_blind_cents": 100,
	}, http.StatusNotFound, nil)

	var overview struct {
		Spaces []store.AdminSpaceSummary `json:"spaces"`
		Users  []store.User              `json:"users"`
	}
	requestJSON(t, manager, http.MethodGet, appServer.URL+"/api/admin/overview", nil, http.StatusOK, &overview)
	if len(overview.Spaces) != 2 || len(overview.Users) != 0 {
		t.Fatalf("channel manager overview leaked data outside its responsibilities: %#v", overview)
	}
	var adminSpaces struct {
		Spaces []store.Space `json:"spaces"`
	}
	requestJSON(t, admin, http.MethodGet, appServer.URL+"/api/spaces", nil, http.StatusOK, &adminSpaces)
	if len(adminSpaces.Spaces) != 3 {
		t.Fatalf("super administrator should see every channel, got %d", len(adminSpaces.Spaces))
	}
}

func (f *fakeNewAPI) serveHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if len(token) > 7 {
		token = token[7:]
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/user/self":
		user, ok := f.users[token]
		if !ok {
			writeFake(w, http.StatusUnauthorized, false, nil)
			return
		}
		copy := make(map[string]any, len(user)+1)
		for key, value := range user {
			copy[key] = value
		}
		copy["quota"] = f.quotas[user["id"].(int64)]
		writeFake(w, http.StatusOK, true, copy)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/user/") && token == "admin-token":
		userID, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/user/"), 10, 64)
		if err != nil {
			writeFake(w, http.StatusBadRequest, false, nil)
			return
		}
		for _, user := range f.users {
			if user["id"].(int64) != userID {
				continue
			}
			copy := make(map[string]any, len(user)+1)
			for key, value := range user {
				copy[key] = value
			}
			copy["quota"] = f.quotas[userID]
			writeFake(w, http.StatusOK, true, copy)
			return
		}
		writeFake(w, http.StatusNotFound, false, nil)
	case r.Method == http.MethodPost && r.URL.Path == "/api/user/manage" && token == "admin-token":
		var input struct {
			ID    int64  `json:"id"`
			Mode  string `json:"mode"`
			Value int64  `json:"value"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		if input.Mode == "add" {
			f.quotas[input.ID] += input.Value
		} else {
			f.quotas[input.ID] -= input.Value
		}
		writeFake(w, http.StatusOK, true, nil)
	default:
		writeFake(w, http.StatusNotFound, false, nil)
	}
}

func writeFake(w http.ResponseWriter, status int, success bool, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": success, "message": "", "data": data})
}

func newTestClient(t *testing.T) *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func openTestDatabase(t *testing.T) *store.Store {
	t.Helper()
	baseURL := os.Getenv("POKERNODE_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("POKERNODE_TEST_DATABASE_URL is required for PostgreSQL app tests")
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
	database, err := store.Open(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func requestJSON(t *testing.T, client *http.Client, method, endpoint string, input any, expectedStatus int, target any) {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != expectedStatus {
		t.Fatalf("%s %s returned %d, want %d: %s", method, endpoint, resp.StatusCode, expectedStatus, body)
	}
	if target != nil && len(body) > 0 {
		if err := json.Unmarshal(body, target); err != nil {
			t.Fatalf("decode response: %v: %s", err, body)
		}
	}
}

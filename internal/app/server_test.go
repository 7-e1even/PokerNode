package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
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
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pokernode/internal/auth"
	"pokernode/internal/landlord"
	"pokernode/internal/poker"
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

func TestUpdateOwnCredentials(t *testing.T) {
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

	alice := newTestClient(t)
	var registered struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, alice, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "alice", "display_name": "Alice", "password": "password-1",
	}, http.StatusCreated, &registered)
	if !registered.User.HasPassword {
		t.Fatal("password registration should report an available password")
	}
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "bob", "display_name": "Bob", "password": "password-2",
	}, http.StatusCreated, nil)
	var profileUpdated struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, alice, http.MethodPatch, appServer.URL+"/api/me/profile", map[string]any{
		"display_name": "Alice Player",
	}, http.StatusOK, &profileUpdated)
	if profileUpdated.User.DisplayName != "Alice Player" {
		t.Fatalf("unexpected updated profile: %#v", profileUpdated.User)
	}
	requestAvatar(t, alice, appServer.URL+"/api/me/avatar", "avatar.txt", []byte("not an image"), http.StatusUnsupportedMediaType, nil)
	pngAvatar := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	requestAvatar(t, alice, appServer.URL+"/api/me/avatar", "avatar.png", pngAvatar, http.StatusOK, &profileUpdated)
	if !strings.HasPrefix(profileUpdated.User.AvatarURL, "/api/users/"+strconv.FormatInt(profileUpdated.User.ID, 10)+"/avatar?v=") {
		t.Fatalf("unexpected avatar URL: %q", profileUpdated.User.AvatarURL)
	}
	avatarResponse, err := alice.Get(appServer.URL + profileUpdated.User.AvatarURL)
	if err != nil {
		t.Fatal(err)
	}
	avatarBody, _ := io.ReadAll(avatarResponse.Body)
	avatarResponse.Body.Close()
	if avatarResponse.StatusCode != http.StatusOK || avatarResponse.Header.Get("Content-Type") != "image/png" || !bytes.Equal(avatarBody, pngAvatar) {
		t.Fatalf("unexpected avatar response: status=%d content-type=%q body=%v", avatarResponse.StatusCode, avatarResponse.Header.Get("Content-Type"), avatarBody)
	}
	var profileDeleted struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, alice, http.MethodDelete, appServer.URL+"/api/me/avatar", nil, http.StatusOK, &profileDeleted)
	if profileDeleted.User.AvatarURL != "" {
		t.Fatalf("avatar URL was not cleared: %q", profileDeleted.User.AvatarURL)
	}

	credentialsURL := appServer.URL + "/api/me/credentials"
	requestJSON(t, alice, http.MethodPatch, credentialsURL, map[string]any{
		"username": "alice_new", "current_password": "wrong-password", "new_password": "new-password-1",
	}, http.StatusUnauthorized, nil)
	requestJSON(t, alice, http.MethodPatch, credentialsURL, map[string]any{
		"username": "bob", "current_password": "password-1", "new_password": "",
	}, http.StatusConflict, nil)
	var updated struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, alice, http.MethodPatch, credentialsURL, map[string]any{
		"username": "alice_new", "current_password": "password-1", "new_password": "new-password-1",
	}, http.StatusOK, &updated)
	if updated.User.Username != "alice_new" || !updated.User.HasPassword {
		t.Fatalf("unexpected updated account: %#v", updated.User)
	}
	requestJSON(t, alice, http.MethodGet, appServer.URL+"/api/me", nil, http.StatusOK, &updated)
	if updated.User.Username != "alice_new" {
		t.Fatalf("updated session did not return the new username: %#v", updated.User)
	}
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{
		"username": "alice", "password": "password-1",
	}, http.StatusUnauthorized, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{
		"username": "alice_new", "password": "new-password-1",
	}, http.StatusOK, nil)

	external, err := database.CreateExternalUser(context.Background(), "wechat", "credential-test-subject", "微信用户", "")
	if err != nil {
		t.Fatal(err)
	}
	externalClient := newTestClient(t)
	sessionValue, _, err := sessions.Issue(external.ID, external.Username)
	if err != nil {
		t.Fatal(err)
	}
	serverURL, err := url.Parse(appServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	externalClient.Jar.SetCookies(serverURL, []*http.Cookie{{Name: auth.CookieName, Value: sessionValue, Path: "/"}})
	requestJSON(t, externalClient, http.MethodPatch, credentialsURL, map[string]any{
		"username": "wechat_user", "current_password": "", "new_password": "",
	}, http.StatusBadRequest, nil)
	requestJSON(t, externalClient, http.MethodPatch, credentialsURL, map[string]any{
		"username": "wechat_user", "current_password": "", "new_password": "wechat-password",
	}, http.StatusOK, &updated)
	if updated.User.Username != "wechat_user" || !updated.User.HasPassword {
		t.Fatalf("external account credentials were not established: %#v", updated.User)
	}
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{
		"username": "wechat_user", "password": "wechat-password",
	}, http.StatusOK, nil)
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
	application := NewServer(database, cipher, sessions, logger)
	appServer := httptest.NewServer(application.Handler(filepath.Join(t.TempDir(), "missing-web")))
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
			ID                   string `json:"id"`
			ActionTimeoutSeconds int    `json:"action_timeout_seconds"`
			Players              []struct {
				Name string `json:"name"`
				Seat int    `json:"seat"`
			} `json:"players"`
		} `json:"tables"`
	}
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	if len(tablesResult.Tables) != 1 || tablesResult.Tables[0].ID != mainTableID || tablesResult.Tables[0].ActionTimeoutSeconds != poker.DefaultActionTimeoutSeconds {
		t.Fatalf("expected the default table, got %#v", tablesResult.Tables)
	}
	var createTableResult struct {
		Table struct {
			ID                   string `json:"id"`
			ActionTimeoutSeconds int    `json:"action_timeout_seconds"`
		} `json:"table"`
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/tables", map[string]any{
		"name": "High Stakes", "small_blind_cents": 100, "big_blind_cents": 200, "action_timeout_seconds": 37,
	}, http.StatusCreated, &createTableResult)
	if createTableResult.Table.ID == "" || createTableResult.Table.ID == mainTableID || createTableResult.Table.ActionTimeoutSeconds != 37 {
		t.Fatalf("expected a second table with a 37 second action timeout, got %#v", createTableResult.Table)
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/tables", map[string]any{
		"name": "Too Fast", "small_blind_cents": 100, "big_blind_cents": 200, "action_timeout_seconds": 4,
	}, http.StatusBadRequest, nil)
	requestJSON(t, alice, http.MethodGet, spacePath+"/tables", nil, http.StatusOK, &tablesResult)
	if len(tablesResult.Tables) != 2 {
		t.Fatalf("expected two tables, got %d", len(tablesResult.Tables))
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/bind", map[string]string{"token": "alice-token"}, http.StatusOK, nil)

	requestJSON(t, bob, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "bob", "display_name": "Bob", "password": "password-2"}, http.StatusCreated, nil)
	requestJSON(t, bob, http.MethodPost, appServer.URL+"/api/spaces/join", map[string]string{"invite_code": createResult.Space.InviteCode}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/bind", map[string]string{"token": "bob-token"}, http.StatusOK, nil)
	var firstMessage struct {
		Message store.SpaceMessage `json:"message"`
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/messages", map[string]string{"body": "今晚八点开局"}, http.StatusCreated, &firstMessage)
	if firstMessage.Message.UserID != 1 || firstMessage.Message.DisplayName != "Alice" || firstMessage.Message.Body != "今晚八点开局" {
		t.Fatalf("unexpected first channel message: %#v", firstMessage.Message)
	}
	requestJSON(t, bob, http.MethodPost, spacePath+"/messages", map[string]string{"body": "  收到  "}, http.StatusCreated, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/messages", map[string]string{"body": ""}, http.StatusBadRequest, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/messages", map[string]string{"body": strings.Repeat("聊", maxSpaceMessageRunes+1)}, http.StatusBadRequest, nil)
	var messageList struct {
		Messages []store.SpaceMessage `json:"messages"`
	}
	requestJSON(t, bob, http.MethodGet, spacePath+"/messages", nil, http.StatusOK, &messageList)
	if len(messageList.Messages) != 2 || messageList.Messages[0].Body != "今晚八点开局" || messageList.Messages[1].Body != "收到" {
		t.Fatalf("channel messages were not returned in order: %#v", messageList.Messages)
	}
	requestJSON(t, bob, http.MethodGet, spacePath+"/messages?after="+strconv.FormatInt(firstMessage.Message.ID, 10), nil, http.StatusOK, &messageList)
	if len(messageList.Messages) != 1 || messageList.Messages[0].Body != "收到" {
		t.Fatalf("channel message cursor returned the wrong messages: %#v", messageList.Messages)
	}
	outsider := newTestClient(t)
	requestJSON(t, outsider, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "outsider", "display_name": "Outsider", "password": "password-3"}, http.StatusCreated, nil)
	requestJSON(t, outsider, http.MethodGet, spacePath+"/messages", nil, http.StatusNotFound, nil)
	requestJSON(t, outsider, http.MethodPost, spacePath+"/messages", map[string]string{"body": "不应发送"}, http.StatusNotFound, nil)
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
			ViewerSeat int    `json:"viewer_seat"`
			ActingSeat int    `json:"acting_seat"`
			Street     string `json:"street"`
		} `json:"table"`
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/ready", map[string]any{}, http.StatusOK, &started)
	if started.Table.Street != "waiting" || started.Table.ActingSeat >= 0 {
		t.Fatalf("one ready player must not start the hand: %#v", started.Table)
	}
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/ready", map[string]any{}, http.StatusOK, nil)
	requestJSON(t, alice, http.MethodGet, spacePath+"/table", nil, http.StatusOK, &started)
	if started.Table.ViewerSeat != started.Table.ActingSeat {
		t.Fatalf("expected Alice to act first heads-up, viewer=%d acting=%d", started.Table.ViewerSeat, started.Table.ActingSeat)
	}
	runtime, err := application.runtimeForTable(context.Background(), createResult.Space.ID, mainTableID)
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	stateData, err := runtime.table.MarshalState()
	if err != nil {
		runtime.mu.Unlock()
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateData, &state); err != nil {
		runtime.mu.Unlock()
		t.Fatal(err)
	}
	state["action_deadline_at"] = time.Now().Add(-time.Second).UnixMilli()
	stateData, err = json.Marshal(state)
	if err != nil {
		runtime.mu.Unlock()
		t.Fatal(err)
	}
	runtime.table, err = poker.RestoreTable(stateData)
	if err != nil {
		runtime.mu.Unlock()
		t.Fatal(err)
	}
	application.syncTableTimerLocked(createResult.Space.ID, mainTableID, runtime)
	runtime.mu.Unlock()

	timeoutDeadline := time.Now().Add(2 * time.Second)
	for {
		requestJSON(t, alice, http.MethodGet, spacePath+"/table", nil, http.StatusOK, &started)
		if started.Table.Street == "complete" {
			break
		}
		if time.Now().After(timeoutDeadline) {
			t.Fatal("server did not apply the expired turn automatically")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var handHistory struct {
		Hands []struct {
			TableID string         `json:"table_id"`
			HandID  int64          `json:"hand_id"`
			Table   poker.Snapshot `json:"table"`
		} `json:"hands"`
	}
	requestJSON(t, alice, http.MethodGet, spacePath+"/hands", nil, http.StatusOK, &handHistory)
	if len(handHistory.Hands) != 1 || handHistory.Hands[0].TableID != mainTableID || handHistory.Hands[0].HandID != 1 || handHistory.Hands[0].Table.LastResult == nil || len(handHistory.Hands[0].Table.LastResult.Players) != 2 {
		t.Fatalf("completed hand history was not returned to Alice: %#v", handHistory.Hands)
	}
	requestJSON(t, bob, http.MethodGet, spacePath+"/hands", nil, http.StatusOK, &handHistory)
	if len(handHistory.Hands) != 1 || handHistory.Hands[0].Table.LastResult == nil {
		t.Fatalf("completed hand history was not returned to Bob: %#v", handHistory.Hands)
	}
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
	requestJSON(t, bob, http.MethodGet, appServer.URL+"/api/leaderboard", nil, http.StatusOK, &leaderboard)
	if len(leaderboard.Entries) != 2 || leaderboard.Entries[0].DisplayName != "Bob" || leaderboard.Entries[1].DisplayName != "Alice" {
		t.Fatalf("unexpected lobby leaderboard: %#v", leaderboard.Entries)
	}
	requestJSON(t, bob, http.MethodPut, appServer.URL+"/api/admin/rankings/1", map[string]bool{"hidden": true}, http.StatusForbidden, nil)
	requestJSON(t, alice, http.MethodPut, appServer.URL+"/api/admin/rankings/2", map[string]bool{"hidden": true}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodGet, spacePath+"/leaderboard", nil, http.StatusOK, &leaderboard)
	if len(leaderboard.Entries) != 1 || leaderboard.Entries[0].DisplayName != "Alice" {
		t.Fatalf("hidden player must not appear in channel leaderboard: %#v", leaderboard.Entries)
	}
	requestJSON(t, bob, http.MethodGet, appServer.URL+"/api/leaderboard", nil, http.StatusOK, &leaderboard)
	if len(leaderboard.Entries) != 1 || leaderboard.Entries[0].DisplayName != "Alice" {
		t.Fatalf("hidden player must not appear in lobby leaderboard: %#v", leaderboard.Entries)
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

func TestPlayerCanOnlyJoinOneTableGlobally(t *testing.T) {
	upstream := &fakeNewAPI{
		users: map[string]map[string]any{
			"admin-token":  {"id": int64(99), "username": "root", "display_name": "Root", "role": 100, "status": 1},
			"player-token": {"id": int64(1), "username": "player-api", "display_name": "Player API", "role": 1, "status": 1},
		},
		quotas: map[int64]int64{1: 100_000_000, 99: 100_000_000},
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
	application := NewServer(database, cipher, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
	appServer := httptest.NewServer(application.Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()
	secondApplication := NewServer(database, cipher, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
	secondAppServer := httptest.NewServer(secondApplication.Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer secondAppServer.Close()

	player := newTestClient(t)
	requestJSON(t, player, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "single_table_player", "display_name": "Player", "password": "password-1",
	}, http.StatusCreated, nil)
	var created struct {
		Space struct {
			ID string `json:"id"`
		} `json:"space"`
	}
	requestJSON(t, player, http.MethodPost, appServer.URL+"/api/spaces", map[string]any{
		"name": "Single table", "newapi_base_url": newAPIServer.URL, "admin_token": "admin-token", "quota_per_usd": 500_000,
	}, http.StatusCreated, &created)
	spacePath := appServer.URL + "/api/spaces/" + created.Space.ID
	requestJSON(t, player, http.MethodPost, spacePath+"/bind", map[string]string{"token": "player-token"}, http.StatusOK, nil)

	var landlordTable struct {
		Table struct {
			ID string `json:"id"`
		} `json:"table"`
	}
	requestJSON(t, player, http.MethodPost, spacePath+"/tables", map[string]any{
		"game_type": "landlord", "name": "Landlord", "base_stake_cents": 100,
	}, http.StatusCreated, &landlordTable)
	secondLandlordPath := secondAppServer.URL + "/api/spaces/" + created.Space.ID + "/tables/" + landlordTable.Table.ID

	var otherSpace struct {
		Space struct {
			ID string `json:"id"`
		} `json:"space"`
	}
	requestJSON(t, player, http.MethodPost, appServer.URL+"/api/spaces", map[string]any{
		"name": "Other channel", "newapi_base_url": newAPIServer.URL, "admin_token": "admin-token", "quota_per_usd": 500_000,
	}, http.StatusCreated, &otherSpace)
	otherSpacePath := appServer.URL + "/api/spaces/" + otherSpace.Space.ID
	requestJSON(t, player, http.MethodPost, otherSpacePath+"/bind", map[string]string{"token": "player-token"}, http.StatusOK, nil)
	var otherLandlord struct {
		Table struct {
			ID string `json:"id"`
		} `json:"table"`
	}
	requestJSON(t, player, http.MethodPost, otherSpacePath+"/tables", map[string]any{
		"game_type": "landlord", "name": "Other landlord", "base_stake_cents": 100,
	}, http.StatusCreated, &otherLandlord)
	otherLandlordPath := secondAppServer.URL + "/api/spaces/" + otherSpace.Space.ID + "/tables/" + otherLandlord.Table.ID
	unseatedMainState, err := database.LoadTableState(context.Background(), created.Space.ID, mainTableID)
	if err != nil {
		t.Fatal(err)
	}

	var mcpKey struct {
		Key string `json:"mcp_key"`
	}
	requestJSON(t, player, http.MethodPost, appServer.URL+"/api/me/mcp-key", nil, http.StatusCreated, &mcpKey)
	mcpSession, err := connectApplicationMCP(appServer.URL+"/mcp", mcpKey.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer mcpSession.Close()
	joinArgs := map[string]any{"space_id": created.Space.ID, "table_id": mainTableID, "buy_in_cents": 2_000}
	if result, callErr := mcpSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_join_table", Arguments: joinArgs}); callErr == nil && !result.IsError {
		t.Fatal("MCP joined before the user enabled Agent control")
	}
	requestJSON(t, player, http.MethodPut, appServer.URL+"/api/me/agent-control", map[string]bool{"enabled": true}, http.StatusOK, nil)
	requestJSON(t, player, http.MethodPost, spacePath+"/table/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusConflict, nil)
	if result, callErr := mcpSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_join_table", Arguments: joinArgs}); callErr != nil || result.IsError {
		t.Fatalf("MCP could not join after handoff: result=%#v err=%v", result, callErr)
	}
	seatedMainState, err := database.LoadTableState(context.Background(), created.Space.ID, mainTableID)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveTableState(context.Background(), created.Space.ID, mainTableID, unseatedMainState); err != nil {
		t.Fatal(err)
	}
	upstream.mu.Lock()
	quotaBeforeRejectedJoin := upstream.quotas[1]
	upstream.mu.Unlock()
	landlordJoinArgs := map[string]any{"space_id": created.Space.ID, "table_id": landlordTable.Table.ID, "buy_in_cents": 2_000}
	if result, callErr := mcpSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_join_table", Arguments: landlordJoinArgs}); callErr == nil && !result.IsError {
		t.Fatal("MCP replaced an active seat when the persisted table snapshot was stale")
	}
	upstream.mu.Lock()
	quotaAfterRejectedJoin := upstream.quotas[1]
	upstream.mu.Unlock()
	if quotaAfterRejectedJoin != quotaBeforeRejectedJoin {
		t.Fatalf("rejected MCP table switch changed quota: before=%d after=%d", quotaBeforeRejectedJoin, quotaAfterRejectedJoin)
	}
	if err := database.SaveTableState(context.Background(), created.Space.ID, mainTableID, seatedMainState); err != nil {
		t.Fatal(err)
	}
	current, callErr := mcpSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_get_current_game", Arguments: map[string]any{}})
	if callErr != nil || current.IsError {
		t.Fatalf("MCP could not discover its current game: result=%#v err=%v", current, callErr)
	}
	encodedCurrent, err := json.Marshal(current.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var currentOutput struct {
		Active  bool   `json:"active"`
		SpaceID string `json:"space_id"`
		TableID string `json:"table_id"`
	}
	if err := json.Unmarshal(encodedCurrent, &currentOutput); err != nil {
		t.Fatal(err)
	}
	if !currentOutput.Active || currentOutput.SpaceID != created.Space.ID || currentOutput.TableID != mainTableID {
		t.Fatalf("unexpected current game: %#v", currentOutput)
	}
	requestJSON(t, player, http.MethodPost, spacePath+"/table/leave", map[string]any{}, http.StatusConflict, nil)
	if result, callErr := mcpSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pokernode_leave_table", Arguments: map[string]any{"space_id": created.Space.ID, "table_id": mainTableID}}); callErr != nil || result.IsError {
		t.Fatalf("MCP could not leave during handoff: result=%#v err=%v", result, callErr)
	}
	requestJSON(t, player, http.MethodPut, appServer.URL+"/api/me/agent-control", map[string]bool{"enabled": false}, http.StatusOK, nil)

	requestJSON(t, player, http.MethodPost, spacePath+"/table/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	requestJSON(t, player, http.MethodPost, secondLandlordPath+"/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusConflict, nil)
	requestJSON(t, player, http.MethodPost, otherLandlordPath+"/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusConflict, nil)
	requestJSON(t, player, http.MethodPost, spacePath+"/table/leave", map[string]any{}, http.StatusOK, nil)
	requestJSON(t, player, http.MethodPost, otherLandlordPath+"/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	requestJSON(t, player, http.MethodPost, otherLandlordPath+"/leave", map[string]any{}, http.StatusOK, nil)

	type joinResult struct {
		leaveEndpoint string
		status        int
		err           error
	}
	join := func(joinEndpoint, leaveEndpoint string, start <-chan struct{}, results chan<- joinResult) {
		<-start
		body, err := json.Marshal(map[string]int64{"buy_in_cents": 2_000})
		if err != nil {
			results <- joinResult{err: err}
			return
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, joinEndpoint, bytes.NewReader(body))
		if err != nil {
			results <- joinResult{err: err}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := player.Do(req)
		if err != nil {
			results <- joinResult{err: err}
			return
		}
		defer resp.Body.Close()
		_, err = io.Copy(io.Discard, resp.Body)
		results <- joinResult{leaveEndpoint: leaveEndpoint, status: resp.StatusCode, err: err}
	}

	start := make(chan struct{})
	results := make(chan joinResult, 2)
	go join(spacePath+"/table/join", spacePath+"/table/leave", start, results)
	go join(otherLandlordPath+"/join", otherLandlordPath+"/leave", start, results)
	close(start)
	concurrentResults := []joinResult{<-results, <-results}
	var successfulLeave string
	conflicts := 0
	for _, result := range concurrentResults {
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch result.status {
		case http.StatusOK:
			if successfulLeave != "" {
				t.Fatalf("both concurrent joins succeeded: %#v", concurrentResults)
			}
			successfulLeave = result.leaveEndpoint
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent join status %d", result.status)
		}
	}
	if successfulLeave == "" || conflicts != 1 {
		t.Fatalf("concurrent joins should produce one success and one conflict: %#v", concurrentResults)
	}

	var seatedTable struct {
		Table struct {
			ViewerSeat int `json:"viewer_seat"`
		} `json:"table"`
	}
	requestJSON(t, player, http.MethodGet, strings.TrimSuffix(successfulLeave, "/leave"), nil, http.StatusOK, &seatedTable)
	if seatedTable.Table.ViewerSeat < 0 {
		t.Fatalf("player was not seated at the table whose concurrent join succeeded: %#v", seatedTable.Table)
	}
	requestJSON(t, player, http.MethodPost, successfulLeave, map[string]any{}, http.StatusOK, nil)

	upstream.mu.Lock()
	playerQuota := upstream.quotas[1]
	upstream.mu.Unlock()
	if playerQuota != 100_000_000 {
		t.Fatalf("joining and leaving tables must preserve the player's balance, got %d", playerQuota)
	}
}

func TestTableKickVoteAndAdminCleanupFlows(t *testing.T) {
	upstream := &fakeNewAPI{
		users: map[string]map[string]any{
			"admin-token": {"id": int64(99), "username": "root", "display_name": "Root", "role": 100, "status": 1},
			"alice-token": {"id": int64(1), "username": "alice-api", "display_name": "Alice API", "role": 1, "status": 1},
			"bob-token":   {"id": int64(2), "username": "bob-api", "display_name": "Bob API", "role": 1, "status": 1},
			"carol-token": {"id": int64(3), "username": "carol-api", "display_name": "Carol API", "role": 1, "status": 1},
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
	application := NewServer(database, cipher, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
	appServer := httptest.NewServer(application.Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()

	type registeredUser struct {
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	register := func(client *http.Client, username, displayName string) int64 {
		t.Helper()
		var result registeredUser
		requestJSON(t, client, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
			"username": username, "display_name": displayName, "password": "password-1",
		}, http.StatusCreated, &result)
		return result.User.ID
	}

	alice := newTestClient(t)
	bob := newTestClient(t)
	carol := newTestClient(t)
	aliceID := register(alice, "alice_vote", "Alice")
	var created struct {
		Space struct {
			ID         string `json:"id"`
			InviteCode string `json:"invite_code"`
		} `json:"space"`
	}
	requestJSON(t, alice, http.MethodPost, appServer.URL+"/api/spaces", map[string]any{
		"name": "Vote table", "newapi_base_url": newAPIServer.URL, "admin_token": "admin-token", "quota_per_usd": 500_000,
	}, http.StatusCreated, &created)
	spacePath := appServer.URL + "/api/spaces/" + created.Space.ID
	requestJSON(t, alice, http.MethodPost, spacePath+"/bind", map[string]string{"token": "alice-token"}, http.StatusOK, nil)

	bobID := register(bob, "bob_vote", "Bob")
	requestJSON(t, bob, http.MethodPost, appServer.URL+"/api/spaces/join", map[string]string{"invite_code": created.Space.InviteCode}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/bind", map[string]string{"token": "bob-token"}, http.StatusOK, nil)
	carolID := register(carol, "carol_vote", "Carol")
	requestJSON(t, carol, http.MethodPost, appServer.URL+"/api/spaces/join", map[string]string{"invite_code": created.Space.InviteCode}, http.StatusOK, nil)
	requestJSON(t, carol, http.MethodPost, spacePath+"/bind", map[string]string{"token": "carol-token"}, http.StatusOK, nil)

	for _, client := range []*http.Client{alice, bob, carol} {
		requestJSON(t, client, http.MethodPost, spacePath+"/table/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/ready", map[string]any{}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/ready", map[string]any{}, http.StatusOK, nil)

	var voteResponse tableEnvelope
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/kick-vote", map[string]any{
		"action": "start", "target_user_id": carolID,
	}, http.StatusOK, &voteResponse)
	if voteResponse.KickVote == nil || voteResponse.KickVote.YesCount != 1 || voteResponse.KickVote.RequiredYes != 2 || voteResponse.KickVote.ViewerVote != "approve" {
		t.Fatalf("unexpected initial kick vote: %#v", voteResponse.KickVote)
	}
	requestJSON(t, carol, http.MethodPost, spacePath+"/table/kick-vote", map[string]string{"action": "approve"}, http.StatusForbidden, nil)
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/kick-vote", map[string]string{"action": "approve"}, http.StatusConflict, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/kick-vote", map[string]string{"action": "reject"}, http.StatusOK, &voteResponse)
	if voteResponse.KickVote != nil || voteResponse.Notice != "同意票不足，移出投票未通过" {
		t.Fatalf("rejected vote should close without removing the player: %#v", voteResponse)
	}

	requestJSON(t, alice, http.MethodPost, spacePath+"/table/kick-vote", map[string]any{
		"action": "start", "target_user_id": carolID,
	}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/kick-vote", map[string]string{"action": "approve"}, http.StatusOK, &voteResponse)
	if voteResponse.KickVote != nil || len(voteResponse.Table.Players) != 2 || !strings.Contains(voteResponse.Notice, "已将 Carol 移出") {
		t.Fatalf("approved vote should remove and settle Carol: %#v", voteResponse)
	}
	for _, player := range voteResponse.Table.Players {
		if player.UserID == carolID || player.Ready {
			t.Fatalf("removed player must be gone and readiness must reset: %#v", voteResponse.Table.Players)
		}
	}
	requestJSON(t, carol, http.MethodPost, spacePath+"/table/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusConflict, nil)

	upstream.mu.Lock()
	carolQuota := upstream.quotas[3]
	upstream.mu.Unlock()
	if carolQuota != 100_000_000 {
		t.Fatalf("vote kick must restore Carol's full buy-in, got quota %d", carolQuota)
	}
	var operations struct {
		Operations []store.WalletOperation `json:"operations"`
	}
	requestJSON(t, carol, http.MethodGet, spacePath+"/operations", nil, http.StatusOK, &operations)
	if len(operations.Operations) != 2 || operations.Operations[0].Kind != "cash_out" || operations.Operations[0].ActorUserID != aliceID || operations.Operations[0].Note != "经牌桌投票移出" {
		t.Fatalf("vote kick cash-out audit is incomplete: %#v", operations.Operations)
	}

	requestJSON(t, alice, http.MethodPost, spacePath+"/table/ready", map[string]any{}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/ready", map[string]any{}, http.StatusOK, &voteResponse)
	if voteResponse.Table.Street != poker.StreetPreflop {
		t.Fatalf("remaining players should be able to start after voting, got %s", voteResponse.Table.Street)
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/clear", map[string]any{}, http.StatusConflict, nil)
	requestJSON(t, alice, http.MethodDelete, spacePath+"/tables/"+mainTableID+"?force=true", nil, http.StatusConflict, nil)
	actingClient := alice
	if voteResponse.Table.Allowed.CanAct {
		actingClient = bob
	}
	requestJSON(t, actingClient, http.MethodPost, spacePath+"/table/action", map[string]any{
		"action": "fold", "expected_turn_id": voteResponse.Table.TurnID + 1,
	}, http.StatusConflict, nil)
	requestJSON(t, actingClient, http.MethodPost, spacePath+"/table/action", map[string]any{
		"action": "fold", "expected_turn_id": voteResponse.Table.TurnID,
	}, http.StatusOK, nil)
	requestJSON(t, alice, http.MethodPost, spacePath+"/table/leave", map[string]any{}, http.StatusOK, nil)
	requestJSON(t, bob, http.MethodPost, spacePath+"/table/leave", map[string]any{}, http.StatusOK, nil)

	var cleanupTable struct {
		Table struct {
			ID string `json:"id"`
		} `json:"table"`
	}
	requestJSON(t, alice, http.MethodPost, spacePath+"/tables", map[string]any{
		"name": "Cleanup", "small_blind_cents": 50, "big_blind_cents": 100,
	}, http.StatusCreated, &cleanupTable)
	cleanupPath := spacePath + "/tables/" + cleanupTable.Table.ID
	for _, client := range []*http.Client{alice, bob, carol} {
		requestJSON(t, client, http.MethodPost, cleanupPath+"/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	}
	requestJSON(t, bob, http.MethodPost, cleanupPath+"/clear", map[string]any{}, http.StatusForbidden, nil)
	var cleanupResponse struct {
		SettledPlayers int          `json:"settled_players"`
		SettledCents   int64        `json:"settled_cents"`
		Table          tableSummary `json:"table"`
	}
	requestJSON(t, alice, http.MethodPost, cleanupPath+"/clear", map[string]any{}, http.StatusOK, &cleanupResponse)
	if cleanupResponse.SettledPlayers != 3 || cleanupResponse.SettledCents != 6_000 || cleanupResponse.Table.PlayerCount != 0 {
		t.Fatalf("admin clear should settle all players and keep an empty table: %#v", cleanupResponse)
	}
	requestJSON(t, carol, http.MethodGet, spacePath+"/operations", nil, http.StatusOK, &operations)
	if operations.Operations[0].Note != "管理员强制清空牌桌" || operations.Operations[0].ActorUserID != aliceID {
		t.Fatalf("admin clear cash-out audit is incomplete: %#v", operations.Operations[0])
	}

	for _, client := range []*http.Client{alice, bob} {
		requestJSON(t, client, http.MethodPost, cleanupPath+"/join", map[string]int64{"buy_in_cents": 2_000}, http.StatusOK, nil)
	}
	requestJSON(t, alice, http.MethodDelete, cleanupPath+"?force=true", nil, http.StatusNoContent, nil)
	requestJSON(t, alice, http.MethodGet, cleanupPath, nil, http.StatusNotFound, nil)
	requestJSON(t, alice, http.MethodGet, spacePath+"/operations", nil, http.StatusOK, &operations)
	if operations.Operations[0].Note != "管理员强制删除牌桌" || operations.Operations[0].ActorUserID != aliceID {
		t.Fatalf("forced table deletion cash-out audit is incomplete: %#v", operations.Operations[0])
	}
	upstream.mu.Lock()
	aliceQuota, bobQuota, finalCarolQuota := upstream.quotas[1], upstream.quotas[2], upstream.quotas[3]
	upstream.mu.Unlock()
	if aliceQuota+bobQuota != 200_000_000 || finalCarolQuota != 100_000_000 {
		t.Fatalf("admin cleanup must preserve balances, got Alice=%d Bob=%d Carol=%d", aliceQuota, bobQuota, finalCarolQuota)
	}
	if aliceID == bobID || bobID == carolID {
		t.Fatal("registered users must have distinct ids")
	}
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

	var publicConfig struct {
		LoginHero struct {
			URL       string  `json:"url"`
			PositionX float64 `json:"position_x"`
			PositionY float64 `json:"position_y"`
			Zoom      float64 `json:"zoom"`
		} `json:"login_hero"`
	}
	requestJSON(t, newTestClient(t), http.MethodGet, appServer.URL+"/api/config", nil, http.StatusOK, &publicConfig)
	if publicConfig.LoginHero.URL != "" || publicConfig.LoginHero.PositionX != 50 || publicConfig.LoginHero.PositionY != 50 || publicConfig.LoginHero.Zoom != 1 {
		t.Fatalf("expected the default login image placeholder, got %#v", publicConfig.LoginHero)
	}
	requestMultipartFile(t, admin, http.MethodPut, appServer.URL+"/api/admin/settings/login-hero", "image", "cover.txt", []byte("not an image"), http.StatusUnsupportedMediaType, nil)
	pngCover := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	requestMultipartFile(t, operator, http.MethodPut, appServer.URL+"/api/admin/settings/login-hero", "image", "cover.png", pngCover, http.StatusForbidden, nil)
	var uploadedCover struct {
		LoginHero struct {
			URL       string  `json:"url"`
			PositionX float64 `json:"position_x"`
			PositionY float64 `json:"position_y"`
			Zoom      float64 `json:"zoom"`
		} `json:"login_hero"`
	}
	requestMultipartFile(t, admin, http.MethodPut, appServer.URL+"/api/admin/settings/login-hero", "image", "cover.png", pngCover, http.StatusOK, &uploadedCover)
	if uploadedCover.LoginHero.URL == "" || uploadedCover.LoginHero.PositionX != 50 || uploadedCover.LoginHero.PositionY != 50 || uploadedCover.LoginHero.Zoom != 1 {
		t.Fatal("expected uploaded login image URL")
	}
	requestJSON(t, operator, http.MethodPatch, appServer.URL+"/api/admin/settings/login-hero", map[string]any{"position_x": 20, "position_y": 70, "zoom": 1.6}, http.StatusForbidden, nil)
	requestJSON(t, admin, http.MethodPatch, appServer.URL+"/api/admin/settings/login-hero", map[string]any{"position_x": -1, "position_y": 70, "zoom": 1.6}, http.StatusBadRequest, nil)
	requestJSON(t, admin, http.MethodPatch, appServer.URL+"/api/admin/settings/login-hero", map[string]any{"position_x": 20, "position_y": 70, "zoom": 1.6}, http.StatusOK, &uploadedCover)
	if uploadedCover.LoginHero.PositionX != 20 || uploadedCover.LoginHero.PositionY != 70 || uploadedCover.LoginHero.Zoom != 1.6 {
		t.Fatalf("login hero placement was not saved: %#v", uploadedCover.LoginHero)
	}
	coverResponse, err := http.Get(appServer.URL + uploadedCover.LoginHero.URL)
	if err != nil {
		t.Fatal(err)
	}
	coverBody, err := io.ReadAll(coverResponse.Body)
	coverResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if coverResponse.StatusCode != http.StatusOK || coverResponse.Header.Get("Content-Type") != "image/png" || coverResponse.Header.Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(coverBody, pngCover) {
		t.Fatalf("unexpected login cover response: status=%d type=%q nosniff=%q body=%v", coverResponse.StatusCode, coverResponse.Header.Get("Content-Type"), coverResponse.Header.Get("X-Content-Type-Options"), coverBody)
	}
	requestJSON(t, operator, http.MethodDelete, appServer.URL+"/api/admin/settings/login-hero", nil, http.StatusForbidden, nil)
	requestJSON(t, admin, http.MethodDelete, appServer.URL+"/api/admin/settings/login-hero", nil, http.StatusNoContent, nil)
	requestJSON(t, newTestClient(t), http.MethodGet, appServer.URL+"/api/config", nil, http.StatusOK, &publicConfig)
	if publicConfig.LoginHero.URL != "" || publicConfig.LoginHero.PositionX != 50 || publicConfig.LoginHero.PositionY != 50 || publicConfig.LoginHero.Zoom != 1 {
		t.Fatalf("expected reset login image config, got %#v", publicConfig.LoginHero)
	}
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

	// 拥有 roles:manage 的账号不得创建、修改、提升或删除超级管理员。
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/roles", map[string]any{
		"key": "role_admin", "name": "角色管理员", "description": "管理角色与账号", "permissions": []string{"admin:view", "users:read", "users:manage", "roles:manage"},
	}, http.StatusCreated, nil)
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{"username": "roleadmin", "display_name": "RoleAdmin", "password": "password-6", "role": "role_admin"}, http.StatusCreated, nil)
	roleAdmin := newTestClient(t)
	requestJSON(t, roleAdmin, http.MethodPost, appServer.URL+"/api/admin/auth/login", map[string]any{"username": "roleadmin", "password": "password-6"}, http.StatusOK, nil)
	requestJSON(t, roleAdmin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{"username": "eviladmin", "display_name": "Evil", "password": "password-7", "role": "super_admin"}, http.StatusForbidden, nil)
	requestJSON(t, roleAdmin, http.MethodPatch, appServer.URL+"/api/admin/users/1", map[string]string{"password": "hacked-password"}, http.StatusForbidden, nil)
	requestJSON(t, roleAdmin, http.MethodPatch, appServer.URL+"/api/admin/users/1", map[string]string{"role": "player", "status": "disabled"}, http.StatusForbidden, nil)
	requestJSON(t, roleAdmin, http.MethodDelete, appServer.URL+"/api/admin/users/1", nil, http.StatusForbidden, nil)
	var pawnResult struct {
		User store.User `json:"user"`
	}
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{"username": "pawn", "display_name": "Pawn", "password": "password-8", "role": "player"}, http.StatusCreated, &pawnResult)
	pawnID := strconv.FormatInt(pawnResult.User.ID, 10)
	requestJSON(t, roleAdmin, http.MethodPatch, appServer.URL+"/api/admin/users/"+pawnID, map[string]string{"role": "super_admin"}, http.StatusForbidden, nil)
	requestJSON(t, roleAdmin, http.MethodPatch, appServer.URL+"/api/admin/users/"+pawnID, map[string]string{"status": "disabled"}, http.StatusOK, nil)
	requestJSON(t, roleAdmin, http.MethodDelete, appServer.URL+"/api/admin/users/"+pawnID, nil, http.StatusNoContent, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/login", map[string]any{"username": "admin", "password": "hacked-password"}, http.StatusUnauthorized, nil)
	// 超级管理员本人仍然可以创建超管账号。
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{"username": "admin2", "display_name": "Admin2", "password": "password-9", "role": "super_admin"}, http.StatusCreated, nil)

	requestJSON(t, admin, http.MethodPut, appServer.URL+"/api/admin/settings/registration", map[string]bool{"enabled": false}, http.StatusOK, nil)
	requestJSON(t, newTestClient(t), http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "blocked", "display_name": "Blocked", "password": "password-4"}, http.StatusForbidden, nil)
}

func TestSuperAdminConfiguresWeChatLogin(t *testing.T) {
	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{15}, 32)))
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
	player := newTestClient(t)
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "wechat_admin", "display_name": "Admin", "password": "password-1"}, http.StatusCreated, nil)
	requestJSON(t, player, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{"username": "wechat_player", "display_name": "Player", "password": "password-2"}, http.StatusCreated, nil)

	var publicConfig struct {
		WeChatLoginEnabled bool `json:"wechat_login_enabled"`
	}
	requestJSON(t, newTestClient(t), http.MethodGet, appServer.URL+"/api/config", nil, http.StatusOK, &publicConfig)
	if publicConfig.WeChatLoginEnabled {
		t.Fatal("wechat login should start disabled")
	}

	settingsPath := appServer.URL + "/api/admin/settings/wechat"
	validSettings := map[string]any{
		"app_id": "wx-test-app-id", "app_secret": "test-wechat-secret",
		"redirect_uri": "https://poker.example/api/auth/wechat/callback", "enabled": true,
	}
	requestJSON(t, player, http.MethodPut, settingsPath, validSettings, http.StatusForbidden, nil)
	requestJSON(t, admin, http.MethodPut, settingsPath, map[string]any{
		"app_id": "wx-test-app-id", "app_secret": "", "redirect_uri": "https://poker.example/api/auth/wechat/callback", "enabled": true,
	}, http.StatusBadRequest, nil)
	requestJSON(t, admin, http.MethodPut, settingsPath, map[string]any{
		"app_id": "wx-test-app-id", "app_secret": "test-wechat-secret", "redirect_uri": "https://poker.example/wrong-callback", "enabled": true,
	}, http.StatusBadRequest, nil)

	var saved map[string]any
	requestJSON(t, admin, http.MethodPut, settingsPath, validSettings, http.StatusOK, &saved)
	encodedSaved, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedSaved), "test-wechat-secret") {
		t.Fatal("wechat app secret leaked in the admin response")
	}
	stored, err := database.WeChatSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stored.AppSecretEnc == "test-wechat-secret" {
		t.Fatal("wechat app secret was stored as plaintext")
	}
	decrypted, err := cipher.Decrypt(stored.AppSecretEnc)
	if err != nil || decrypted != "test-wechat-secret" {
		t.Fatalf("stored wechat app secret could not be decrypted: %v", err)
	}
	requestJSON(t, newTestClient(t), http.MethodGet, appServer.URL+"/api/config", nil, http.StatusOK, &publicConfig)
	if !publicConfig.WeChatLoginEnabled {
		t.Fatal("wechat login did not become active without a restart")
	}

	noRedirect := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	startResponse, err := noRedirect.Get(appServer.URL + "/api/auth/wechat/start")
	if err != nil {
		t.Fatal(err)
	}
	startResponse.Body.Close()
	if startResponse.StatusCode != http.StatusFound {
		t.Fatalf("wechat start returned %d", startResponse.StatusCode)
	}
	authorizeURL, err := url.Parse(startResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if authorizeURL.Query().Get("appid") != "wx-test-app-id" || authorizeURL.Query().Get("redirect_uri") != "https://poker.example/api/auth/wechat/callback" {
		t.Fatalf("unexpected wechat authorize URL: %s", authorizeURL)
	}

	reloadedServer := httptest.NewServer(NewServer(database, cipher, sessions, logger).Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer reloadedServer.Close()
	requestJSON(t, newTestClient(t), http.MethodGet, reloadedServer.URL+"/api/config", nil, http.StatusOK, &publicConfig)
	if !publicConfig.WeChatLoginEnabled {
		t.Fatal("stored wechat settings were not loaded after restart")
	}

	requestJSON(t, admin, http.MethodPut, settingsPath, map[string]any{
		"app_id": "wx-test-app-id-2", "app_secret": "", "redirect_uri": "https://poker.example/api/auth/wechat/callback", "enabled": true,
	}, http.StatusOK, nil)
	stored, err = database.WeChatSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err = cipher.Decrypt(stored.AppSecretEnc)
	if err != nil || decrypted != "test-wechat-secret" {
		t.Fatalf("blank secret did not preserve the existing encrypted secret: %v", err)
	}

	requestJSON(t, admin, http.MethodPut, settingsPath, map[string]any{
		"app_id": "wx-test-app-id-2", "app_secret": "", "redirect_uri": "https://poker.example/api/auth/wechat/callback", "enabled": false,
	}, http.StatusOK, nil)
	requestJSON(t, newTestClient(t), http.MethodGet, appServer.URL+"/api/config", nil, http.StatusOK, &publicConfig)
	if publicConfig.WeChatLoginEnabled {
		t.Fatal("wechat login remained active after being disabled")
	}
}

func TestSuperAdminForceDeleteUserAndNewAPIAccount(t *testing.T) {
	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{12}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions("test-session-secret-that-is-long-enough", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var deletedMu sync.Mutex
	var deletedNewAPIUserID int64
	newAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/user/77" || r.Header.Get("Authorization") != "Bearer admin-token" {
			writeFake(w, http.StatusBadRequest, false, nil)
			return
		}
		deletedMu.Lock()
		deletedNewAPIUserID = 77
		deletedMu.Unlock()
		writeFake(w, http.StatusOK, true, nil)
	}))
	defer newAPIServer.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	appServer := httptest.NewServer(NewServer(database, cipher, sessions, logger).Handler(filepath.Join(t.TempDir(), "missing-web")))
	defer appServer.Close()
	admin := newTestClient(t)
	var adminResult struct {
		User store.User `json:"user"`
	}
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/auth/register", map[string]any{
		"username": "force_admin", "display_name": "Force Admin", "password": "password-1",
	}, http.StatusCreated, &adminResult)
	var targetResult struct {
		User store.User `json:"user"`
	}
	requestJSON(t, admin, http.MethodPost, appServer.URL+"/api/admin/users", map[string]any{
		"username": "force_player", "display_name": "Force Player", "password": "password-2", "role": "player",
	}, http.StatusCreated, &targetResult)
	adminTokenEnc, err := cipher.Encrypt("admin-token")
	if err != nil {
		t.Fatal(err)
	}
	userTokenEnc, err := cipher.Encrypt("user-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSpace(context.Background(), store.Space{
		ID: "force-space", Name: "Force Space", InviteCode: "FORCE1", OwnerUserID: targetResult.User.ID,
		BaseURL: newAPIServer.URL, AdminTokenEnc: adminTokenEnc, AdminTokenLast4: "oken",
		AdminNewAPIUserID: 99, AdminNewAPIRole: 100, QuotaPerUSD: 500_000, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.BindMember(context.Background(), store.Member{
		SpaceID: "force-space", UserID: targetResult.User.ID, NewAPIUserID: 77, NewAPIUsername: "force-api",
		NewAPIDisplay: "Force API", NewAPIRole: 1, UserTokenEnc: userTokenEnc, UserTokenLast4: "oken",
	}); err != nil {
		t.Fatal(err)
	}

	table := poker.NewTable("occupied", "Occupied", 50, 100)
	if _, err := table.Join(targetResult.User.ID, targetResult.User.DisplayName, 1_000); err != nil {
		t.Fatal(err)
	}
	state, err := table.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveTableState(context.Background(), "force-space", "occupied", state); err != nil {
		t.Fatal(err)
	}
	emptyLandlordTable := landlord.NewTable("empty-landlord", "Empty Landlord", 100)
	landlordState, err := emptyLandlordTable.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveTableState(context.Background(), "force-space", "empty-landlord", landlordState); err != nil {
		t.Fatal(err)
	}
	forceURL := appServer.URL + "/api/admin/users/" + strconv.FormatInt(targetResult.User.ID, 10) + "?force=true"
	requestJSON(t, admin, http.MethodDelete, forceURL, nil, http.StatusConflict, nil)
	deletedMu.Lock()
	if deletedNewAPIUserID != 0 {
		deletedMu.Unlock()
		t.Fatal("New API account was deleted while the player was still seated")
	}
	deletedMu.Unlock()
	if _, err := table.Leave(targetResult.User.ID); err != nil {
		t.Fatal(err)
	}
	state, err = table.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveTableState(context.Background(), "force-space", "occupied", state); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, admin, http.MethodDelete, forceURL, nil, http.StatusNoContent, nil)
	deletedMu.Lock()
	if deletedNewAPIUserID != 77 {
		deletedMu.Unlock()
		t.Fatalf("New API account deletion was not called, got %d", deletedNewAPIUserID)
	}
	deletedMu.Unlock()
	deletedUser, err := database.UserByID(context.Background(), targetResult.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deletedUser.Status != "deleted" || deletedUser.Username == targetResult.User.Username {
		t.Fatalf("local account was not anonymized: %#v", deletedUser)
	}
	space, err := database.SpaceByID(context.Background(), "force-space")
	if err != nil {
		t.Fatal(err)
	}
	if space.OwnerUserID != adminResult.User.ID {
		t.Fatalf("owned channel was not transferred to the deleting administrator: %#v", space)
	}
	var overview struct {
		Users []store.User `json:"users"`
	}
	requestJSON(t, admin, http.MethodGet, appServer.URL+"/api/admin/overview", nil, http.StatusOK, &overview)
	for _, user := range overview.Users {
		if user.ID == targetResult.User.ID {
			t.Fatalf("forced-deleted user remained visible: %#v", user)
		}
	}
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

func requestAvatar(t *testing.T, client *http.Client, endpoint, filename string, data []byte, expectedStatus int, target any) {
	t.Helper()
	requestMultipartFile(t, client, http.MethodPut, endpoint, "avatar", filename, data, expectedStatus, target)
}

func requestMultipartFile(t *testing.T, client *http.Client, method, endpoint, fieldName, filename string, data []byte, expectedStatus int, target any) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, endpoint, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != expectedStatus {
		t.Fatalf("%s %s returned %d, want %d: %s", method, endpoint, resp.StatusCode, expectedStatus, responseBody)
	}
	if target != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, target); err != nil {
			t.Fatalf("decode response: %v: %s", err, responseBody)
		}
	}
}

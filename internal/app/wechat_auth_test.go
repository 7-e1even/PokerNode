package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"pokernode/internal/auth"
	"pokernode/internal/secure"
	"pokernode/internal/store"
	"pokernode/internal/wechat"
)

type fakeWeChatProvider struct {
	profile wechat.Profile
}

func (f *fakeWeChatProvider) AuthorizeURL(redirectURL, state string) string {
	values := url.Values{"redirect_uri": {redirectURL}, "state": {state}}
	return "https://wechat.example/authorize?" + values.Encode()
}

func (f *fakeWeChatProvider) Authenticate(_ context.Context, _ string) (wechat.Profile, error) {
	return f.profile, nil
}

func TestWeChatAutoRegistrationAndExistingLogin(t *testing.T) {
	provider := &fakeWeChatProvider{profile: wechat.Profile{OpenID: "open-alice", UnionID: "union-alice", Nickname: "微信 Alice"}}
	appServer, database := newWeChatTestServer(t, provider)
	defer appServer.Close()
	defer database.Close()

	firstClient := newTestClient(t)
	completeWeChatAuth(t, firstClient, appServer.URL, "/api/auth/wechat/start", "wechat-code", "")
	var firstSession struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, firstClient, http.MethodGet, appServer.URL+"/api/me", nil, http.StatusOK, &firstSession)
	if firstSession.User.DisplayName != "微信 Alice" || firstSession.User.Role != "super_admin" || !firstSession.User.WeChatBound {
		t.Fatalf("unexpected auto-registered user: %#v", firstSession.User)
	}

	secondClient := newTestClient(t)
	completeWeChatAuth(t, secondClient, appServer.URL, "/api/auth/wechat/start", "wechat-code-again", "")
	var secondSession struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, secondClient, http.MethodGet, appServer.URL+"/api/me", nil, http.StatusOK, &secondSession)
	if secondSession.User.ID != firstSession.User.ID {
		t.Fatalf("existing wechat identity created another user: first=%d second=%d", firstSession.User.ID, secondSession.User.ID)
	}
}

func TestExistingUserCanBindWeChat(t *testing.T) {
	provider := &fakeWeChatProvider{profile: wechat.Profile{OpenID: "open-legacy", Nickname: "Legacy WeChat"}}
	appServer, database := newWeChatTestServer(t, provider)
	defer appServer.Close()
	defer database.Close()

	legacyClient := newTestClient(t)
	var registered struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, legacyClient, http.MethodPost, appServer.URL+"/api/auth/register", map[string]string{
		"username": "legacy", "display_name": "Legacy", "password": "legacy-password",
	}, http.StatusCreated, &registered)
	completeWeChatAuth(t, legacyClient, appServer.URL, "/api/auth/wechat/link", "link-code", "success")

	var linked struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, legacyClient, http.MethodGet, appServer.URL+"/api/me", nil, http.StatusOK, &linked)
	if linked.User.ID != registered.User.ID || !linked.User.WeChatBound {
		t.Fatalf("legacy user was not linked: %#v", linked.User)
	}

	wechatClient := newTestClient(t)
	completeWeChatAuth(t, wechatClient, appServer.URL, "/api/auth/wechat/start", "login-code", "")
	var wechatSession struct {
		User authenticatedUser `json:"user"`
	}
	requestJSON(t, wechatClient, http.MethodGet, appServer.URL+"/api/me", nil, http.StatusOK, &wechatSession)
	if wechatSession.User.ID != registered.User.ID {
		t.Fatalf("linked wechat login returned user %d, want %d", wechatSession.User.ID, registered.User.ID)
	}
}

func TestWeChatCannotBeBoundToTwoUsers(t *testing.T) {
	provider := &fakeWeChatProvider{profile: wechat.Profile{OpenID: "shared-openid", Nickname: "Shared"}}
	appServer, database := newWeChatTestServer(t, provider)
	defer appServer.Close()
	defer database.Close()

	first := newTestClient(t)
	second := newTestClient(t)
	requestJSON(t, first, http.MethodPost, appServer.URL+"/api/auth/register", map[string]string{
		"username": "first_user", "display_name": "First", "password": "first-password",
	}, http.StatusCreated, nil)
	requestJSON(t, second, http.MethodPost, appServer.URL+"/api/auth/register", map[string]string{
		"username": "second_user", "display_name": "Second", "password": "second-password",
	}, http.StatusCreated, nil)
	completeWeChatAuth(t, first, appServer.URL, "/api/auth/wechat/link", "first-code", "success")
	completeWeChatAuth(t, second, appServer.URL, "/api/auth/wechat/link", "second-code", "already_bound")
}

func newWeChatTestServer(t *testing.T, provider wechatProvider) (*httptest.Server, *store.Store) {
	t.Helper()
	database := openTestDatabase(t)
	cipher, err := secure.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{13}, 32)))
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions("test-session-secret-that-is-long-enough", time.Hour)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewServer(database, cipher, sessions, logger, WithWeChat("https://poker.example/api/auth/wechat/callback", provider)).Handler(filepath.Join(t.TempDir(), "missing-web")))
	return server, database
}

func completeWeChatAuth(t *testing.T, client *http.Client, serverURL, startPath, code, expectedResult string) {
	t.Helper()
	originalRedirect := client.CheckRedirect
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { client.CheckRedirect = originalRedirect }()

	startResponse, err := client.Get(serverURL + startPath)
	if err != nil {
		t.Fatal(err)
	}
	startResponse.Body.Close()
	if startResponse.StatusCode != http.StatusFound {
		t.Fatalf("start wechat auth returned %d", startResponse.StatusCode)
	}
	authorizeURL, err := url.Parse(startResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := authorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("wechat authorization URL is missing state")
	}

	callbackResponse, err := client.Get(serverURL + "/api/auth/wechat/callback?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	callbackResponse.Body.Close()
	if callbackResponse.StatusCode != http.StatusFound {
		t.Fatalf("wechat callback returned %d", callbackResponse.StatusCode)
	}
	location, err := url.Parse(callbackResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if expectedResult == "" {
		if location.Path != "/" || location.RawQuery != "" {
			t.Fatalf("wechat login redirected to %q", location.String())
		}
		return
	}
	if result := location.Query().Get("wechat_link"); result != expectedResult {
		t.Fatalf("wechat link result is %q, want %q", result, expectedResult)
	}
}

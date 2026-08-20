package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authorizeEndpoint = "https://open.weixin.qq.com/connect/qrconnect"
	tokenEndpoint     = "https://api.weixin.qq.com/sns/oauth2/access_token"
	profileEndpoint   = "https://api.weixin.qq.com/sns/userinfo"
)

type Profile struct {
	OpenID    string
	UnionID   string
	Nickname  string
	AvatarURL string
}

func (p Profile) Subject() string {
	if p.OpenID != "" {
		return "openid:" + p.OpenID
	}
	if p.UnionID != "" {
		return "unionid:" + p.UnionID
	}
	return ""
}

type Client struct {
	appID      string
	appSecret  string
	httpClient *http.Client
}

func NewClient(appID, appSecret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{appID: appID, appSecret: appSecret, httpClient: httpClient}
}

func (c *Client) AuthorizeURL(redirectURL, state string) string {
	values := url.Values{
		"appid":         {c.appID},
		"redirect_uri":  {redirectURL},
		"response_type": {"code"},
		"scope":         {"snsapi_login"},
		"state":         {state},
	}
	return authorizeEndpoint + "?" + values.Encode() + "#wechat_redirect"
}

func (c *Client) Authenticate(ctx context.Context, code string) (Profile, error) {
	values := url.Values{
		"appid":      {c.appID},
		"secret":     {c.appSecret},
		"code":       {code},
		"grant_type": {"authorization_code"},
	}
	var token tokenResponse
	if err := c.getJSON(ctx, tokenEndpoint+"?"+values.Encode(), &token); err != nil {
		return Profile{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	if token.ErrCode != 0 {
		return Profile{}, fmt.Errorf("exchange authorization code: wechat error %d: %s", token.ErrCode, token.ErrMsg)
	}
	if token.AccessToken == "" || token.OpenID == "" {
		return Profile{}, fmt.Errorf("exchange authorization code: incomplete response")
	}

	values = url.Values{
		"access_token": {token.AccessToken},
		"openid":       {token.OpenID},
		"lang":         {"zh_CN"},
	}
	var result profileResponse
	if err := c.getJSON(ctx, profileEndpoint+"?"+values.Encode(), &result); err != nil {
		return Profile{}, fmt.Errorf("load user profile: %w", err)
	}
	if result.ErrCode != 0 {
		return Profile{}, fmt.Errorf("load user profile: wechat error %d: %s", result.ErrCode, result.ErrMsg)
	}
	if result.OpenID == "" {
		return Profile{}, fmt.Errorf("load user profile: incomplete response")
	}
	return Profile{OpenID: result.OpenID, UnionID: result.UnionID, Nickname: result.Nickname, AvatarURL: normalizeAvatarURL(result.AvatarURL)}, nil
}

func normalizeAvatarURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") {
		return "https://" + strings.TrimPrefix(value, "http://")
	}
	return value
}

func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	OpenID      string `json:"openid"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type profileResponse struct {
	OpenID    string `json:"openid"`
	UnionID   string `json:"unionid"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"headimgurl"`
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
}

package newapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const DefaultQuotaPerUSD int64 = 500_000

type Client struct {
	http *http.Client
}

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
	Status      int    `json:"status"`
	Quota       int64  `json:"quota"`
}

type response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 12 * time.Second}}
}

func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("New API 地址无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("New API 地址只支持 http 或 https")
	}
	if parsed.User != nil {
		return "", errors.New("New API 地址不能包含用户名或密码")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func (c *Client) Self(ctx context.Context, baseURL, token string) (User, error) {
	return getUser(ctx, c.http, baseURL+"/api/user/self", token)
}

func (c *Client) User(ctx context.Context, baseURL, adminToken string, userID int64) (User, error) {
	return getUser(ctx, c.http, fmt.Sprintf("%s/api/user/%d", baseURL, userID), adminToken)
}

func (c *Client) DeleteUser(ctx context.Context, baseURL, adminToken string, userID int64) error {
	if userID <= 0 {
		return errors.New("New API 用户 ID 无效")
	}
	var result response[json.RawMessage]
	if err := doJSON(ctx, c.http, http.MethodDelete, fmt.Sprintf("%s/api/user/%d", baseURL, userID), adminToken, nil, &result); err != nil {
		return err
	}
	if !result.Success {
		return messageError(result.Message)
	}
	return nil
}

// ProvisionUser creates a normal New API user, logs in as that user and
// generates the long-lived System Access Token PokerNode needs for balance
// operations. Re-running it with the same credentials is safe: if the user
// already exists, the login step recovers the existing account.
func (c *Client) ProvisionUser(ctx context.Context, baseURL, adminToken, username, password, displayName string) (User, string, error) {
	createErr := c.createUser(ctx, baseURL, adminToken, username, password, displayName)
	sessionClient, sessionToken, loginUser, err := c.login(ctx, baseURL, username, password)
	if err != nil {
		if createErr != nil {
			return User{}, "", fmt.Errorf("创建 New API 用户失败: %v；自动登录失败: %w", createErr, err)
		}
		return User{}, "", fmt.Errorf("自动登录 New API 用户失败: %w", err)
	}

	var tokenResult response[string]
	if err := doJSON(ctx, sessionClient, http.MethodGet, baseURL+"/api/user/token", sessionToken, nil, &tokenResult); err != nil {
		return User{}, "", err
	}
	if !tokenResult.Success || strings.TrimSpace(tokenResult.Data) == "" {
		return User{}, "", messageError(tokenResult.Message)
	}
	token := strings.TrimSpace(tokenResult.Data)
	verified, err := c.Self(ctx, baseURL, token)
	if err != nil {
		return User{}, "", fmt.Errorf("验证自动生成的 System Access Token 失败: %w", err)
	}
	if loginUser.ID > 0 && verified.ID != loginUser.ID {
		return User{}, "", errors.New("New API 自动生成的 Token 用户不一致")
	}
	if verified.Username != username {
		return User{}, "", errors.New("New API 自动创建的用户名不一致")
	}
	return verified, token, nil
}

func (c *Client) createUser(ctx context.Context, baseURL, adminToken, username, password, displayName string) error {
	body := map[string]any{
		"username": username, "password": password, "display_name": displayName, "role": 1,
	}
	var result response[json.RawMessage]
	if err := doJSON(ctx, c.http, http.MethodPost, baseURL+"/api/user/", adminToken, body, &result); err != nil {
		return err
	}
	if !result.Success {
		return messageError(result.Message)
	}
	return nil
}

func (c *Client) login(ctx context.Context, baseURL, username, password string) (*http.Client, string, User, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "", User{}, err
	}
	client := &http.Client{Timeout: 12 * time.Second, Jar: jar}
	var result response[json.RawMessage]
	if err := doJSON(ctx, client, http.MethodPost, baseURL+"/api/user/login", "", map[string]string{
		"username": username, "password": password,
	}, &result); err != nil {
		return nil, "", User{}, err
	}
	if !result.Success {
		return nil, "", User{}, messageError(result.Message)
	}

	var modern struct {
		AccessToken string `json:"access_token"`
		User        User   `json:"user"`
	}
	_ = json.Unmarshal(result.Data, &modern)
	if modern.User.ID > 0 {
		return client, strings.TrimSpace(modern.AccessToken), modern.User, nil
	}
	var legacy User
	if err := json.Unmarshal(result.Data, &legacy); err != nil || legacy.ID <= 0 {
		return nil, "", User{}, errors.New("New API 登录返回了无效用户")
	}
	return client, strings.TrimSpace(modern.AccessToken), legacy, nil
}

func getUser(ctx context.Context, client *http.Client, endpoint, token string) (User, error) {
	var result response[User]
	if err := doJSON(ctx, client, http.MethodGet, endpoint, token, nil, &result); err != nil {
		return User{}, err
	}
	if !result.Success {
		return User{}, messageError(result.Message)
	}
	if result.Data.ID <= 0 || result.Data.Username == "" {
		return User{}, errors.New("New API 返回了无效用户")
	}
	return result.Data, nil
}

func (c *Client) AdjustQuota(ctx context.Context, baseURL, adminToken string, userID, quota int64, add bool) error {
	if quota <= 0 {
		return errors.New("quota adjustment must be positive")
	}
	mode := "subtract"
	if add {
		mode = "add"
	}
	body := map[string]any{"id": userID, "action": "add_quota", "mode": mode, "value": quota}
	var result response[any]
	if err := doJSON(ctx, c.http, http.MethodPost, baseURL+"/api/user/manage", adminToken, body, &result); err != nil {
		return err
	}
	if !result.Success {
		return messageError(result.Message)
	}
	return nil
}

func doJSON(ctx context.Context, client *http.Client, method, endpoint, token string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接 New API 失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("New API 返回 HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("New API 返回内容无法解析")
	}
	return nil
}

func messageError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "New API 请求失败"
	}
	return errors.New(message)
}

func CentsToQuota(cents, quotaPerUSD int64) (int64, error) {
	if cents <= 0 {
		return 0, errors.New("amount and quota conversion must be positive")
	}
	maxCents, err := MaxCentsForQuota(quotaPerUSD)
	if err != nil {
		return 0, err
	}
	if cents > maxCents {
		return 0, errors.New("quota conversion is too large")
	}
	return cents * (quotaPerUSD / 100), nil
}

func MaxCentsForQuota(quotaPerUSD int64) (int64, error) {
	if quotaPerUSD <= 0 {
		return 0, errors.New("amount and quota conversion must be positive")
	}
	if quotaPerUSD%100 != 0 {
		return 0, errors.New("quota_per_usd must be divisible by 100")
	}
	perCent := quotaPerUSD / 100
	return math.MaxInt64 / perCent, nil
}

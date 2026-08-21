package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"pokernode/internal/auth"
	"pokernode/internal/landlord"
	"pokernode/internal/poker"
	"pokernode/internal/store"
)

const defaultBaseURL = "http://127.0.0.1:8080"

type Config struct {
	BaseURL           string
	Username          string
	Password          string
	SessionToken      string
	MCPKey            string
	AllowInsecureHTTP bool
}

func ConfigFromEnv() (Config, error) {
	config := Config{
		BaseURL:      strings.TrimSpace(os.Getenv("POKERNODE_BASE_URL")),
		Username:     strings.TrimSpace(os.Getenv("POKERNODE_USERNAME")),
		Password:     os.Getenv("POKERNODE_PASSWORD"),
		SessionToken: strings.TrimSpace(os.Getenv("POKERNODE_SESSION_TOKEN")),
		MCPKey:       strings.TrimSpace(os.Getenv("POKERNODE_MCP_KEY")),
	}
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if value := strings.TrimSpace(os.Getenv("POKERNODE_ALLOW_INSECURE_HTTP")); value != "" {
		allow, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("POKERNODE_ALLOW_INSECURE_HTTP must be true or false")
		}
		config.AllowInsecureHTTP = allow
	}
	return config, config.Validate()
}

func (c Config) Validate() error {
	baseURL, err := url.Parse(c.BaseURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return errors.New("POKERNODE_BASE_URL must be an absolute http or https URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return errors.New("POKERNODE_BASE_URL must not contain credentials, a query, or a fragment")
	}
	if baseURL.Scheme == "http" && !isLoopbackHost(baseURL.Hostname()) && !c.AllowInsecureHTTP {
		return errors.New("remote PokerNode URLs must use HTTPS; set POKERNODE_ALLOW_INSECURE_HTTP=true only for a trusted private network")
	}
	if c.MCPKey == "" && c.SessionToken == "" && (c.Username == "" || c.Password == "") {
		return errors.New("set POKERNODE_MCP_KEY, POKERNODE_SESSION_TOKEN, or both POKERNODE_USERNAME and POKERNODE_PASSWORD")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type APIClient struct {
	baseURL       *url.URL
	httpClient    *http.Client
	username      string
	password      string
	sessionToken  string
	mcpKey        string
	authMu        sync.Mutex
	authenticated bool
}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("PokerNode API returned %d: %s", e.Status, e.Message)
}

type channelView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsBound   bool   `json:"is_bound"`
	CanManage bool   `json:"can_manage"`
}

type tableSummary struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	GameType             string `json:"game_type"`
	SmallBlind           int64  `json:"small_blind_cents,omitempty" jsonschema:"Integer USD cents; 50 means 0.50 USD."`
	BigBlind             int64  `json:"big_blind_cents,omitempty" jsonschema:"Integer USD cents; 100 means 1.00 USD."`
	BaseStake            int64  `json:"base_stake_cents,omitempty" jsonschema:"Integer USD cents; 100 means 1.00 USD."`
	ActionTimeoutSeconds int    `json:"action_timeout_seconds"`
	PlayerCount          int    `json:"player_count"`
	MaxPlayers           int    `json:"max_players"`
	HandID               int64  `json:"hand_id"`
	Street               string `json:"street"`
	ViewerSeated         bool   `json:"viewer_seated"`
}

type tableEnvelope struct {
	Table    gameSnapshot
	KickVote *wireKickVote
	Notice   string
}

type gameSnapshot struct {
	GameType string
	Poker    *poker.Snapshot
	Landlord *landlord.Snapshot
}

type wireTableEnvelope struct {
	Table       json.RawMessage `json:"table"`
	KickVote    *wireKickVote   `json:"kick_vote"`
	Notice      string          `json:"notice,omitempty"`
	OperationID string          `json:"operation_id,omitempty"`
	Settled     int64           `json:"settled_cents,omitempty"`
}

type wireKickVote struct {
	TargetUserID  int64  `json:"target_user_id"`
	TargetName    string `json:"target_name"`
	InitiatorName string `json:"initiator_name"`
	YesCount      int    `json:"yes_count"`
	RequiredYes   int    `json:"required_yes"`
	ExpiresAt     int64  `json:"expires_at"`
}

type wireCurrentGame struct {
	Active              bool            `json:"active"`
	AgentControlEnabled bool            `json:"agent_control_enabled"`
	SpaceID             string          `json:"space_id,omitempty"`
	TableID             string          `json:"table_id,omitempty"`
	Table               json.RawMessage `json:"table,omitempty"`
}

type currentGame struct {
	Active              bool
	AgentControlEnabled bool
	SpaceID             string
	TableID             string
	Table               gameSnapshot
}

type gameActionRequest struct {
	Action         string
	Amount         int64
	Bid            int
	Cards          []landlord.Card
	ExpectedTurnID uint64
}

func NewAPIClient(config Config, httpClient *http.Client) (*APIClient, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	baseURL, _ := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if httpClient == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("create cookie jar: %w", err)
		}
		httpClient = &http.Client{Jar: jar, Timeout: 30 * time.Second}
	} else if httpClient.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("create cookie jar: %w", err)
		}
		httpClient.Jar = jar
	}
	client := &APIClient{
		baseURL: baseURL, httpClient: httpClient, username: config.Username,
		password: config.Password, sessionToken: config.SessionToken, mcpKey: config.MCPKey,
	}
	if config.MCPKey != "" {
		client.authenticated = true
	} else if config.SessionToken != "" {
		httpClient.Jar.SetCookies(baseURL, []*http.Cookie{{Name: auth.CookieName, Value: config.SessionToken, Path: "/"}})
		client.authenticated = true
	}
	return client, nil
}

func (c *APIClient) listChannels(ctx context.Context) ([]channelView, error) {
	var response struct {
		Spaces []store.Space `json:"spaces"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "api/spaces", nil, &response); err != nil {
		return nil, err
	}
	channels := make([]channelView, 0, len(response.Spaces))
	for _, space := range response.Spaces {
		channels = append(channels, channelView{ID: space.ID, Name: space.Name, IsBound: space.IsBound, CanManage: space.CanManage})
	}
	return channels, nil
}

func (c *APIClient) listTables(ctx context.Context, spaceID string) ([]tableSummary, error) {
	var response struct {
		Tables []tableSummary `json:"tables"`
	}
	path := fmt.Sprintf("api/spaces/%s/tables", url.PathEscape(spaceID))
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Tables, nil
}

func (c *APIClient) getCurrentGame(ctx context.Context) (currentGame, error) {
	var response wireCurrentGame
	if err := c.doJSON(ctx, http.MethodGet, "api/me/current-game", nil, &response); err != nil {
		return currentGame{}, err
	}
	current := currentGame{Active: response.Active, AgentControlEnabled: response.AgentControlEnabled, SpaceID: response.SpaceID, TableID: response.TableID}
	if !response.Active {
		return current, nil
	}
	table, err := decodeGameSnapshot(response.Table)
	current.Table = table
	return current, err
}

func (c *APIClient) getTable(ctx context.Context, spaceID, tableID string) (tableEnvelope, error) {
	var response wireTableEnvelope
	if err := c.doJSON(ctx, http.MethodGet, tablePath(spaceID, tableID), nil, &response); err != nil {
		return tableEnvelope{}, err
	}
	table, err := decodeGameSnapshot(response.Table)
	return tableEnvelope{Table: table, KickVote: response.KickVote, Notice: response.Notice}, err
}

func (c *APIClient) joinTable(ctx context.Context, spaceID, tableID string, buyIn int64) (gameSnapshot, string, error) {
	var response wireTableEnvelope
	if err := c.doJSON(ctx, http.MethodPost, tablePath(spaceID, tableID)+"/join", map[string]int64{"buy_in_cents": buyIn}, &response); err != nil {
		return gameSnapshot{}, "", err
	}
	table, err := decodeGameSnapshot(response.Table)
	return table, response.OperationID, err
}

func (c *APIClient) ready(ctx context.Context, spaceID, tableID string) (tableEnvelope, error) {
	var response wireTableEnvelope
	if err := c.doJSON(ctx, http.MethodPost, tablePath(spaceID, tableID)+"/ready", struct{}{}, &response); err != nil {
		return tableEnvelope{}, err
	}
	table, err := decodeGameSnapshot(response.Table)
	return tableEnvelope{Table: table, KickVote: response.KickVote, Notice: response.Notice}, err
}

func (c *APIClient) act(ctx context.Context, spaceID, tableID string, input gameActionRequest) (tableEnvelope, error) {
	var response wireTableEnvelope
	body := struct {
		Action         string          `json:"action"`
		Amount         int64           `json:"amount_cents,omitempty"`
		Bid            int             `json:"bid,omitempty"`
		Cards          []landlord.Card `json:"cards,omitempty"`
		ExpectedTurnID uint64          `json:"expected_turn_id"`
	}{Action: input.Action, Amount: input.Amount, Bid: input.Bid, Cards: input.Cards, ExpectedTurnID: input.ExpectedTurnID}
	if err := c.doJSON(ctx, http.MethodPost, tablePath(spaceID, tableID)+"/action", body, &response); err != nil {
		return tableEnvelope{}, err
	}
	table, err := decodeGameSnapshot(response.Table)
	return tableEnvelope{Table: table, KickVote: response.KickVote, Notice: response.Notice}, err
}

func (c *APIClient) leaveTable(ctx context.Context, spaceID, tableID string) (gameSnapshot, string, int64, error) {
	var response wireTableEnvelope
	if err := c.doJSON(ctx, http.MethodPost, tablePath(spaceID, tableID)+"/leave", struct{}{}, &response); err != nil {
		return gameSnapshot{}, "", 0, err
	}
	table, err := decodeGameSnapshot(response.Table)
	return table, response.OperationID, response.Settled, err
}

func decodeGameSnapshot(data json.RawMessage) (gameSnapshot, error) {
	var header struct {
		GameType string `json:"game_type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return gameSnapshot{}, fmt.Errorf("decode table type: %w", err)
	}
	if header.GameType == landlord.GameType {
		var snapshot landlord.Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return gameSnapshot{}, fmt.Errorf("decode landlord table: %w", err)
		}
		return gameSnapshot{GameType: landlord.GameType, Landlord: &snapshot}, nil
	}
	if header.GameType != "" && header.GameType != poker.GameType {
		return gameSnapshot{}, fmt.Errorf("unsupported game type %q", header.GameType)
	}
	var snapshot poker.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return gameSnapshot{}, fmt.Errorf("decode poker table: %w", err)
	}
	return gameSnapshot{GameType: poker.GameType, Poker: &snapshot}, nil
}

func tablePath(spaceID, tableID string) string {
	return fmt.Sprintf("api/spaces/%s/tables/%s", url.PathEscape(spaceID), url.PathEscape(tableID))
}

func (c *APIClient) doJSON(ctx context.Context, method, path string, body, target any) error {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return err
	}
	status, err := c.requestJSON(ctx, method, path, body, target)
	if status != http.StatusUnauthorized || c.sessionToken != "" || c.mcpKey != "" {
		return err
	}
	c.authMu.Lock()
	c.authenticated = false
	c.authMu.Unlock()
	if loginErr := c.ensureAuthenticated(ctx); loginErr != nil {
		return loginErr
	}
	_, err = c.requestJSON(ctx, method, path, body, target)
	return err
}

func (c *APIClient) ensureAuthenticated(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.authenticated {
		return nil
	}
	if c.mcpKey != "" {
		c.authenticated = true
		return nil
	}
	var response struct {
		User json.RawMessage `json:"user"`
	}
	body := map[string]string{"username": c.username, "password": c.password}
	if _, err := c.requestJSON(ctx, http.MethodPost, "api/auth/login", body, &response); err != nil {
		return fmt.Errorf("log in to PokerNode: %w", err)
	}
	c.authenticated = true
	return nil
}

func (c *APIClient) requestJSON(ctx context.Context, method, path string, body, target any) (int, error) {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("encode PokerNode request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	endpoint, err := url.JoinPath(c.baseURL.String(), path)
	if err != nil {
		return 0, fmt.Errorf("build PokerNode URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return 0, fmt.Errorf("create PokerNode request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "pokernode-mcp/0.1")
	if c.mcpKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.mcpKey)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("contact PokerNode at %s: %w", c.baseURL.Redacted(), err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return response.StatusCode, fmt.Errorf("read PokerNode response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &payload)
		if payload.Error == "" {
			payload.Error = http.StatusText(response.StatusCode)
		}
		return response.StatusCode, &apiError{Status: response.StatusCode, Message: payload.Error}
	}
	if target == nil || len(bytes.TrimSpace(data)) == 0 {
		return response.StatusCode, nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return response.StatusCode, fmt.Errorf("decode PokerNode response: %w", err)
	}
	return response.StatusCode, nil
}

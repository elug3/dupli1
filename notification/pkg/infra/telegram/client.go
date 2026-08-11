package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const apiBase = "https://api.telegram.org"

// Client sends messages via the Telegram Bot API.
type Client struct {
	token      string
	httpClient *http.Client
	apiBase    string
	policy     AccessPolicy
}

// NewClient creates a Telegram notifier. When token is empty the client is a no-op.
func NewClient(token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
	}
}

// NewTestClient creates a client that talks to a custom API base (for unit tests).
func NewTestClient(token string, httpClient *http.Client, apiBaseURL string) *Client {
	c := NewClient(token, httpClient)
	c.apiBase = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	return c
}

func (c *Client) baseURL() string {
	if c != nil && c.apiBase != "" {
		return c.apiBase
	}
	return apiBase
}

func (c *Client) Enabled() bool {
	return c != nil && c.token != ""
}

// SetAccessPolicy configures which chats and users may receive outbound messages and commands.
func (c *Client) SetAccessPolicy(policy AccessPolicy) {
	if c != nil {
		c.policy = policy
	}
}

// SetAllowlist configures which chats may receive outbound messages.
// Deprecated: use SetAccessPolicy.
func (c *Client) SetAllowlist(allowlist *Allowlist) {
	if c != nil && allowlist != nil {
		c.policy = allowlist
	}
}

// Send posts a text message to the given chat ID.
// When an access policy is set, chats that are not allowlisted are skipped (no error).
func (c *Client) Send(ctx context.Context, chatID string, message string) error {
	return c.sendMessage(ctx, chatID, message, true)
}

// Reply posts a command reply (e.g. /start ack) without applying the outbound chat allowlist.
// Pending registrations are not allowlisted yet, so ops acks must bypass AllowsChat.
func (c *Client) Reply(ctx context.Context, chatID string, message string) error {
	return c.sendMessage(ctx, chatID, message, false)
}

func (c *Client) sendMessage(ctx context.Context, chatID string, message string, enforcePolicy bool) error {
	if c == nil || c.token == "" {
		return nil
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return fmt.Errorf("telegram chat id is required")
	}
	if enforcePolicy && c.policy != nil && !c.policy.AllowsChat(chatID) {
		return nil
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("telegram message is required")
	}

	body, err := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	})
	if err != nil {
		return fmt.Errorf("marshal telegram request: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL(), c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && !result.OK {
		return fmt.Errorf("telegram api returned ok=false: %s", strings.TrimSpace(string(respBody)))
	}

	return nil
}

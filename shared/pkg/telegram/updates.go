package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ErrUpdatesConflict reports that another consumer owns this bot's update
// stream — a second polling task, or a webhook that is still registered.
// Telegram answers getUpdates with 409 Conflict in both cases, and retrying
// hard does not help: the other consumer has to go away first.
var ErrUpdatesConflict = errors.New("telegram getUpdates conflict")

// User is a Telegram user who sent a message.
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

// Chat is a Telegram chat from an incoming update.
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Message is an incoming Telegram message.
type Message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from"`
}

// CallbackQuery is an inline-keyboard button tap.
//
// Message is the message the button hangs under — the bot's own — so its
// MessageID is what EditMessageText needs to walk a menu in place. Data is the
// button's callback_data, capped by Telegram at 64 bytes.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// Chat reports the chat a button tap came from, or the zero Chat when the
// update carries no message (Telegram omits it for very old messages).
func (q CallbackQuery) Chat() Chat {
	if q.Message == nil {
		return Chat{}
	}
	return q.Message.Chat
}

// Update is a Telegram Bot API update. Exactly one of the pointer fields is
// set; a handler checks the one it serves and ignores the rest.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

func (c Chat) FormatID() string {
	return strconv.FormatInt(c.ID, 10)
}

// DeleteWebhook clears any webhook so getUpdates long-polling works.
func (c *Client) DeleteWebhook(ctx context.Context) error {
	if c == nil || c.token == "" {
		return nil
	}

	url := fmt.Sprintf("%s/bot%s/deleteWebhook", c.baseURL(), c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create deleteWebhook request: %w", c.redact(err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete telegram webhook: %w", c.redact(err))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram deleteWebhook status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

const (
	// updateBatchLimit caps how many updates Telegram returns per call. The
	// default of 100, each up to a 4096-character message, can exceed any
	// sensible read budget in one response.
	updateBatchLimit = 20
	// maxUpdatesBody bounds the response read. It is generous next to
	// updateBatchLimit: a body that reaches it is reported as an error rather
	// than silently truncated, because truncated JSON fails to decode, the
	// offset never advances, and the poller then retries the same window
	// forever.
	maxUpdatesBody = 4 << 20
)

// GetUpdates fetches pending updates. timeout is the long-poll seconds (0–50).
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	if c == nil || c.token == "" {
		return nil, nil
	}

	url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=%d&limit=%d", c.baseURL(), c.token, offset, timeout, updateBatchLimit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create getUpdates request: %w", c.redact(err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get telegram updates: %w", c.redact(err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxUpdatesBody+1))
	if err != nil {
		return nil, fmt.Errorf("read telegram updates: %w", c.redact(err))
	}
	if len(respBody) > maxUpdatesBody {
		return nil, fmt.Errorf("telegram getUpdates response exceeds %d bytes", maxUpdatesBody)
	}
	if resp.StatusCode == http.StatusConflict {
		return nil, fmt.Errorf("%w: %s", ErrUpdatesConflict, strings.TrimSpace(string(respBody)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram getUpdates status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode telegram updates: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram getUpdates returned ok=false: %s", strings.TrimSpace(string(respBody)))
	}

	return result.Result, nil
}

// WebhookOption customizes webhook registration.
type WebhookOption func(*webhookConfig)

type webhookConfig struct {
	allowedUpdates []string
}

// WithAllowedUpdates names the update types Telegram should deliver. A bot with
// menus needs "callback_query" alongside "message": Telegram filters anything
// not listed here, so an unlisted button tap is dropped before it is ever sent
// and the button spins on the user's device forever.
//
// The default stays "message" alone, which is what an alert-only bot wants and
// what the ops bot has always registered.
func WithAllowedUpdates(types ...string) WebhookOption {
	return func(cfg *webhookConfig) {
		cfg.allowedUpdates = types
	}
}

// SetWebhook registers the bot webhook URL and optional secret token.
func (c *Client) SetWebhook(ctx context.Context, webhookURL, secretToken string, opts ...WebhookOption) error {
	if c == nil || c.token == "" {
		return nil
	}
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return fmt.Errorf("telegram webhook url is required")
	}

	cfg := webhookConfig{allowedUpdates: []string{"message"}}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	payload := map[string]any{
		"url":                  webhookURL,
		"allowed_updates":      cfg.allowedUpdates,
		"drop_pending_updates": false,
	}
	if secret := strings.TrimSpace(secretToken); secret != "" {
		payload["secret_token"] = secret
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal setWebhook request: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/setWebhook", c.baseURL(), c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create setWebhook request: %w", c.redact(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("set telegram webhook: %w", c.redact(err))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram setWebhook status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

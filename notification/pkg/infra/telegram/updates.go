package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// User is a Telegram user who sent a message.
type User struct {
	ID int64 `json:"id"`
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

// Update is a Telegram Bot API update.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
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
		return fmt.Errorf("create deleteWebhook request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete telegram webhook: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram deleteWebhook status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

// GetUpdates fetches pending updates. timeout is the long-poll seconds (0–50).
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	if c == nil || c.token == "" {
		return nil, nil
	}

	url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=%d", c.baseURL(), c.token, offset, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create getUpdates request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get telegram updates: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if err != nil {
		return nil, fmt.Errorf("read telegram updates: %w", err)
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

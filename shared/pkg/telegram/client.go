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

// Retry budget for a failed send. Vars, not consts, so tests need not wait out
// the real backoff.
var (
	sendMaxAttempts  = 3
	sendRetryBackoff = 500 * time.Millisecond
)

// tokenPlaceholder stands in for the bot token in redacted error text.
const tokenPlaceholder = "<redacted>"

// redactedError hides the bot token in a wrapped error's message.
//
// Every Bot API URL carries the token in its path (/bot<TOKEN>/sendMessage),
// and net/http reports transport failures as *url.Error, whose Error() embeds
// the full request URL. Logging such an error verbatim — which is exactly what
// the NATS dispatcher and the poller do — publishes the token to CloudWatch.
type redactedError struct {
	err   error
	token string
}

func (e *redactedError) Error() string {
	return strings.ReplaceAll(e.err.Error(), e.token, tokenPlaceholder)
}

func (e *redactedError) Unwrap() error { return e.err }

// redact wraps err so the bot token never reaches a log line. Callers must pass
// every error that may carry a Bot API URL through here before wrapping it.
func (c *Client) redact(err error) error {
	if err == nil || c == nil || c.token == "" {
		return err
	}
	return &redactedError{err: err, token: c.token}
}

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

// Send posts a text message to the given chat ID.
// When an access policy is set, chats that are not allowlisted are skipped (no error).
func (c *Client) Send(ctx context.Context, chatID string, message string) error {
	return c.sendMessage(ctx, chatID, message, true, nil)
}

// Reply posts a command reply (e.g. /start ack) without applying the outbound chat allowlist.
// Pending registrations are not allowlisted yet, so ops acks must bypass AllowsChat.
//
// The same reasoning covers every answer to something the chat just did —
// AnswerCallback and EditMessageText below — because a chat the bot is
// mid-conversation with must hear back whether or not it is allowlisted.
func (c *Client) Reply(ctx context.Context, chatID string, message string) error {
	return c.sendMessage(ctx, chatID, message, false, nil)
}

// ReplyMenu is Reply with an inline keyboard attached — the opening menu of a
// consultation, sent in answer to /start.
//
// It bypasses the outbound allowlist for the same reason Reply does. There is
// deliberately no policy-enforced variant: a menu is always an answer to
// something the chat just did, never an unsolicited push. Add one when a bot
// genuinely needs to start a conversation with buttons.
func (c *Client) ReplyMenu(ctx context.Context, chatID string, message string, markup *InlineKeyboardMarkup) error {
	return c.sendMessage(ctx, chatID, message, false, markup)
}

// EditMessageText replaces the text (and any keyboard) of a message the bot
// already sent. A menu walks in place with this rather than stacking a new
// message per tap, which is what keeps a consultation readable on a phone.
//
// A nil markup clears the buttons: Telegram treats an absent reply_markup on
// an edit as "no keyboard", which is exactly what a final answer wants.
func (c *Client) EditMessageText(ctx context.Context, chatID string, messageID int64, message string, markup *InlineKeyboardMarkup) error {
	if c == nil || c.token == "" {
		return nil
	}
	if messageID == 0 {
		return fmt.Errorf("telegram message id is required")
	}
	payload, err := c.buildMessage(chatID, message, false, markup)
	if err != nil || payload == nil {
		return err
	}
	payload.MessageID = messageID

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram request: %w", err)
	}
	return c.callWithRetry(ctx, "editMessageText", body)
}

// AnswerCallback acknowledges a button tap. Telegram shows a loading spinner on
// the tapped button until this is called, so every CallbackQuery needs one even
// when there is nothing to say — text may be empty, which just dismisses it.
//
// Like Reply, this bypasses the outbound allowlist: it answers an interaction
// the chat itself started.
func (c *Client) AnswerCallback(ctx context.Context, callbackQueryID string, text string) error {
	if c == nil || c.token == "" {
		return nil
	}
	callbackQueryID = strings.TrimSpace(callbackQueryID)
	if callbackQueryID == "" {
		return fmt.Errorf("telegram callback query id is required")
	}

	payload := map[string]string{"callback_query_id": callbackQueryID}
	if text = strings.TrimSpace(text); text != "" {
		payload["text"] = text
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram request: %w", err)
	}
	return c.callWithRetry(ctx, "answerCallbackQuery", body)
}

func (c *Client) sendMessage(ctx context.Context, chatID string, message string, enforcePolicy bool, markup *InlineKeyboardMarkup) error {
	if c == nil || c.token == "" {
		return nil
	}
	payload, err := c.buildMessage(chatID, message, enforcePolicy, markup)
	if err != nil || payload == nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram request: %w", err)
	}
	return c.callWithRetry(ctx, "sendMessage", body)
}

// buildMessage validates and assembles a message body. A nil payload with a nil
// error means the access policy refused this chat, which Send reports as
// success — a skipped alert is not a failure for the caller to retry.
func (c *Client) buildMessage(chatID string, message string, enforcePolicy bool, markup *InlineKeyboardMarkup) (*messagePayload, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("telegram chat id is required")
	}
	if enforcePolicy && c.policy != nil && !c.policy.AllowsChat(chatID) {
		return nil, nil
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, fmt.Errorf("telegram message is required")
	}

	return &messagePayload{
		ChatID:      chatID,
		Text:        truncateMessage(message),
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	}, nil
}

// callWithRetry posts body to one Bot API method, repeating a failure that is
// worth repeating.
func (c *Client) callWithRetry(ctx context.Context, method string, body []byte) error {
	var lastErr error
	backoff := sendRetryBackoff
	for attempt := 1; ; attempt++ {
		retryable, err := c.postMessage(ctx, method, body)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || attempt >= sendMaxAttempts {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

// postMessage performs one Bot API call and reports whether the failure is
// worth repeating. A rejected chat or a malformed message fails the same way
// every time; a timeout or a 5xx usually does not.
//
// The retry runs inline, so a NATS handler waits out the backoff — bounded by
// sendMaxAttempts, and worth it because core NATS does not redeliver: without a
// retry here, one blip loses an alert for good, order.paid included.
func (c *Client) postMessage(ctx context.Context, method string, body []byte) (retryable bool, err error) {
	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL(), c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("create telegram request: %w", c.redact(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Transport-level: timeout, reset, DNS. Worth another attempt unless
		// the caller's context is the thing that ended.
		return ctx.Err() == nil, fmt.Errorf("send telegram message: %w", c.redact(err))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return retryable, fmt.Errorf("telegram api status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && !result.OK {
		return false, fmt.Errorf("telegram api returned ok=false: %s", strings.TrimSpace(string(respBody)))
	}

	return false, nil
}

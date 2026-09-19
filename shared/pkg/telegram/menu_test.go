package telegram_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/shared/pkg/telegram"
)

// capture records the Bot API method and decoded body of each call, which is
// what these tests assert on: the wire format is the contract with Telegram.
type capture struct {
	methods []string
	bodies  []map[string]any
	raw     []string
}

func fakeAPI(t *testing.T, status int) (*telegram.Client, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		got.methods = append(got.methods, parts[len(parts)-1])

		var buf strings.Builder
		var body map[string]any
		dec := json.NewDecoder(io.TeeReader(r.Body, &buf))
		if err := dec.Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		got.bodies = append(got.bodies, body)
		got.raw = append(got.raw, buf.String())

		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	return telegram.NewTestClient("test-token", srv.Client(), srv.URL), got
}

func TestSendWithoutKeyboardKeepsTheOriginalWireFormat(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	if err := client.Send(t.Context(), "42", "주문 도착"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got.methods[0] != "sendMessage" {
		t.Fatalf("method = %q, want sendMessage", got.methods[0])
	}
	// The payload became a struct so reply_markup could exist. A plain alert
	// must still serialize to exactly the three keys it always had — an extra
	// null reply_markup is rejected by Telegram as invalid markup.
	if _, ok := got.bodies[0]["reply_markup"]; ok {
		t.Fatalf("plain message must not carry reply_markup: %s", got.raw[0])
	}
	if len(got.bodies[0]) != 3 {
		t.Fatalf("keys = %v, want exactly chat_id/text/parse_mode", got.bodies[0])
	}
	if got.bodies[0]["parse_mode"] != "HTML" || got.bodies[0]["text"] != "주문 도착" {
		t.Fatalf("body = %v", got.bodies[0])
	}
}

func TestReplyMenuAttachesInlineKeyboard(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	menu := telegram.KeyboardRows(
		telegram.CallbackButton("📦 주문·배송 문의", "v1:ord"),
		telegram.CallbackButton("🙋 상담원 연결", "v1:agt"),
	)
	if err := client.ReplyMenu(t.Context(), "42", "무엇을 도와드릴까요?", menu); err != nil {
		t.Fatalf("ReplyMenu: %v", err)
	}

	var body struct {
		ReplyMarkup struct {
			InlineKeyboard [][]struct {
				Text         string `json:"text"`
				CallbackData string `json:"callback_data"`
			} `json:"inline_keyboard"`
		} `json:"reply_markup"`
	}
	if err := json.Unmarshal([]byte(got.raw[0]), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rows := body.ReplyMarkup.InlineKeyboard
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (one button per row)", len(rows))
	}
	if len(rows[0]) != 1 || rows[0][0].Text != "📦 주문·배송 문의" || rows[0][0].CallbackData != "v1:ord" {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1][0].CallbackData != "v1:agt" {
		t.Fatalf("second row = %+v", rows[1])
	}
}

func TestKeyboardRowsIsNilForNoButtons(t *testing.T) {
	// A menu built from an empty list must not send an empty keyboard, which
	// Telegram renders as a blank button strip.
	if telegram.KeyboardRows() != nil {
		t.Fatal("empty keyboard must be nil")
	}
	client, got := fakeAPI(t, http.StatusOK)
	if err := client.ReplyMenu(t.Context(), "42", "hi", telegram.KeyboardRows()); err != nil {
		t.Fatalf("ReplyMenu: %v", err)
	}
	if _, ok := got.bodies[0]["reply_markup"]; ok {
		t.Fatalf("nil keyboard must not reach the wire: %s", got.raw[0])
	}
}

func TestEditMessageTextWalksAMenuInPlace(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	next := telegram.KeyboardRows(telegram.CallbackButton("⬅️ 처음으로", "v1:root"))
	if err := client.EditMessageText(t.Context(), "42", 777, "배송은 2–3일 걸립니다.", next); err != nil {
		t.Fatalf("EditMessageText: %v", err)
	}

	if got.methods[0] != "editMessageText" {
		t.Fatalf("method = %q, want editMessageText", got.methods[0])
	}
	if got.bodies[0]["message_id"] != float64(777) {
		t.Fatalf("message_id = %v, want 777", got.bodies[0]["message_id"])
	}
	if got.bodies[0]["chat_id"] != "42" {
		t.Fatalf("chat_id = %v", got.bodies[0]["chat_id"])
	}
	if _, ok := got.bodies[0]["reply_markup"]; !ok {
		t.Fatalf("edit must carry the next keyboard: %s", got.raw[0])
	}
}

func TestEditMessageTextWithoutKeyboardClearsTheButtons(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	if err := client.EditMessageText(t.Context(), "42", 777, "상담이 종료되었습니다.", nil); err != nil {
		t.Fatalf("EditMessageText: %v", err)
	}
	// Telegram reads an absent reply_markup on an edit as "no keyboard", which
	// is how a final answer drops the menu it replaced.
	if _, ok := got.bodies[0]["reply_markup"]; ok {
		t.Fatalf("final answer must clear the keyboard: %s", got.raw[0])
	}
}

func TestEditMessageTextRequiresAMessageID(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	err := client.EditMessageText(t.Context(), "42", 0, "text", nil)
	if err == nil {
		t.Fatal("expected an error for a zero message id")
	}
	if len(got.methods) != 0 {
		t.Fatalf("nothing should reach the API, got %v", got.methods)
	}
}

func TestAnswerCallbackDismissesTheSpinner(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	if err := client.AnswerCallback(t.Context(), "cbq-1", ""); err != nil {
		t.Fatalf("AnswerCallback: %v", err)
	}

	if got.methods[0] != "answerCallbackQuery" {
		t.Fatalf("method = %q", got.methods[0])
	}
	if got.bodies[0]["callback_query_id"] != "cbq-1" {
		t.Fatalf("callback_query_id = %v", got.bodies[0]["callback_query_id"])
	}
	if _, ok := got.bodies[0]["text"]; ok {
		t.Fatalf("empty text must be omitted, not sent blank: %s", got.raw[0])
	}
}

func TestAnswerCallbackCarriesText(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	if err := client.AnswerCallback(t.Context(), "cbq-1", "접수되었습니다"); err != nil {
		t.Fatalf("AnswerCallback: %v", err)
	}
	if got.bodies[0]["text"] != "접수되었습니다" {
		t.Fatalf("text = %v", got.bodies[0]["text"])
	}
}

func TestAnswerCallbackRequiresAnID(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	if err := client.AnswerCallback(t.Context(), "  ", "hi"); err == nil {
		t.Fatal("expected an error for a blank callback query id")
	}
	if len(got.methods) != 0 {
		t.Fatalf("nothing should reach the API, got %v", got.methods)
	}
}

func TestMidConversationAnswersBypassTheOutboundAllowlist(t *testing.T) {
	// Send is a broadcast and obeys the policy; an answer to something the chat
	// just did must land regardless, or a pending chat never hears back.
	client, got := fakeAPI(t, http.StatusOK)
	client.SetAccessPolicy(allowChat("-1001"))

	if err := client.Send(t.Context(), "42", "blocked"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(got.methods) != 0 {
		t.Fatalf("Send must skip a non-allowlisted chat, got %v", got.methods)
	}

	if err := client.EditMessageText(t.Context(), "42", 5, "allowed", nil); err != nil {
		t.Fatalf("EditMessageText: %v", err)
	}
	if err := client.AnswerCallback(t.Context(), "cbq-1", ""); err != nil {
		t.Fatalf("AnswerCallback: %v", err)
	}
	if len(got.methods) != 2 {
		t.Fatalf("methods = %v, want the edit and the callback answer", got.methods)
	}
}

func TestEditMessageTextTruncatesLikeSend(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	if err := client.EditMessageText(t.Context(), "42", 5, strings.Repeat("가", 5000), nil); err != nil {
		t.Fatalf("EditMessageText: %v", err)
	}
	text, _ := got.bodies[0]["text"].(string)
	if n := len([]rune(text)); n > 4096 {
		t.Fatalf("edited text = %d runes, want the 4096 limit applied", n)
	}
}

func TestNewMethodsRetryServerErrors(t *testing.T) {
	client, got := fakeAPI(t, http.StatusInternalServerError)

	if err := client.AnswerCallback(t.Context(), "cbq-1", ""); err == nil {
		t.Fatal("expected the 500 to surface after the attempt budget")
	}
	if len(got.methods) < 2 {
		t.Fatalf("attempts = %d, want the retry budget spent", len(got.methods))
	}
}

func TestUpdateDecodesACallbackQuery(t *testing.T) {
	raw := `{"update_id":7,"callback_query":{"id":"cbq-1","from":{"id":99,"username":"shopper"},
	         "message":{"message_id":555,"chat":{"id":-1001,"type":"private"}},"data":"v1:ord"}}`

	var update telegram.Update
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if update.Message != nil {
		t.Fatal("a callback query update carries no message of its own")
	}
	q := update.CallbackQuery
	if q == nil || q.ID != "cbq-1" || q.Data != "v1:ord" {
		t.Fatalf("callback query = %+v", q)
	}
	if q.From == nil || q.From.ID != 99 {
		t.Fatalf("from = %+v", q.From)
	}
	// The message id is what EditMessageText needs to replace the menu.
	if q.Message.MessageID != 555 {
		t.Fatalf("message id = %d, want 555", q.Message.MessageID)
	}
	if q.Chat().FormatID() != "-1001" {
		t.Fatalf("chat = %q", q.Chat().FormatID())
	}
}

func TestCallbackQueryChatIsZeroSafe(t *testing.T) {
	// Telegram omits the message for taps on very old messages.
	q := telegram.CallbackQuery{ID: "cbq-1", Data: "v1:ord"}
	if got := q.Chat().FormatID(); got != "0" {
		t.Fatalf("chat id = %q, want the zero chat", got)
	}
}

func TestSetWebhookRegistersMessagesOnlyByDefault(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	if err := client.SetWebhook(t.Context(), "https://example.test/hook", "secret"); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}
	allowed, _ := got.bodies[0]["allowed_updates"].([]any)
	if len(allowed) != 1 || allowed[0] != "message" {
		t.Fatalf("allowed_updates = %v, want the unchanged ops-bot default", allowed)
	}
}

func TestSetWebhookCanOptIntoCallbackQueries(t *testing.T) {
	client, got := fakeAPI(t, http.StatusOK)

	err := client.SetWebhook(t.Context(), "https://example.test/hook", "secret",
		telegram.WithAllowedUpdates("message", "callback_query"))
	if err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}
	allowed, _ := got.bodies[0]["allowed_updates"].([]any)
	if len(allowed) != 2 || allowed[1] != "callback_query" {
		t.Fatalf("allowed_updates = %v, want message and callback_query", allowed)
	}
}

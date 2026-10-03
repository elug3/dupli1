package handler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/handler"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	"github.com/elug3/dupli1/support/pkg/livefeed"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

// webValidator reads "user|perms|account_type" so a test can be a customer,
// a manager, or a service account.
type webValidator struct{}

func (webValidator) ValidateAccessToken(token string) (authjwt.Claims, error) {
	parts := strings.SplitN(token, "|", 3)
	claims := authjwt.Claims{UserID: parts[0], Email: parts[0] + "@example.com", AccountType: "customer"}
	if len(parts) > 1 && parts[1] != "" {
		claims.Permissions = strings.Split(parts[1], ",")
	}
	if len(parts) > 2 && parts[2] != "" {
		claims.AccountType = parts[2]
	}
	return claims, nil
}

type webProducts struct{}

func (webProducts) Variant(_ context.Context, skuID string) (*ports.ProductRef, error) {
	if skuID != "SKU01" {
		return nil, ports.ErrReferenceNotFound
	}
	return &ports.ProductRef{ProductID: "P01", SkuID: "SKU01", Name: "Prada Galleria", PriceWon: 3200000}, nil
}

type webOrders struct{}

func (webOrders) Order(_ context.Context, bearer, orderID string) (*ports.OrderRef, error) {
	if orderID == "ORD1" && strings.HasPrefix(bearer, "user-1|") {
		return &ports.OrderRef{OrderID: "ORD1", Status: "paid", TotalWon: 3200000}, nil
	}
	return nil, ports.ErrReferenceNotFound
}

type webFixture struct {
	mux    *http.ServeMux
	server *httptest.Server
	hub    *livefeed.Hub
}

func newWebFixture(t *testing.T) *webFixture {
	t.Helper()
	conversations := memory.NewConversationRepository()
	inquiries := memory.NewInquiryRepository()
	messages := memory.NewMessageRepository()
	hub := livefeed.NewHub()
	now := time.Date(2026, 9, 17, 2, 0, 0, 0, time.UTC) // 11:00 KST, Thursday
	n := 0
	newID := func() string {
		n++
		return "id-" + strings.Repeat("0", 3) + string(rune('a'+n%26)) + string(rune('a'+n/26))
	}
	clock := func() time.Time { return now }

	chat := service.NewWebChat(service.WebChatDeps{
		Conversations: conversations, Inquiries: inquiries, Messages: messages,
		Live: hub, Products: webProducts{}, Orders: webOrders{},
		Hours: domain.DefaultBusinessHours(), NewID: newID, Now: clock,
	})
	inbox := service.NewInbox(conversations, inquiries, messages, &recordingBot{}, newID, clock).
		WithWebChat(webProducts{}, webOrders{}, hub)

	h := handler.New(handler.Options{
		Inbox:        inbox,
		Answers:      memory.NewAnswerRepository(),
		JWTValidator: webValidator{},
		Settings:     settings.NewResponse("support"),
		WebChat:      chat,
		Hub:          hub,
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return &webFixture{mux: mux, server: server, hub: hub}
}

const (
	customerToken = "user-1||customer"
	otherToken    = "user-2||customer"
	staffToken    = "manager-1|" + permissions.SupportReply + "|manager"
)

func TestWebChatRoundTrip(t *testing.T) {
	f := newWebFixture(t)

	res := do(t, f.mux, http.MethodGet, "/api/v1/support/web/conversation", customerToken, "")
	if res.Code != http.StatusOK {
		t.Fatalf("empty conversation: %d %s", res.Code, res.Body)
	}

	res = do(t, f.mux, http.MethodPost, "/api/v1/support/web/messages", customerToken,
		`{"body":"재입고 되나요?","product_id":"P01","sku_id":"SKU01"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("send: %d %s", res.Code, res.Body)
	}
	var sent struct {
		Conversation struct {
			Inquiry struct {
				ID    string `json:"id"`
				SkuID string `json:"sku_id"`
			} `json:"inquiry"`
			Messages []struct {
				Kind   string          `json:"kind"`
				Body   string          `json:"body"`
				Ref    json.RawMessage `json:"ref"`
				Author string          `json:"author"`
			} `json:"messages"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &sent); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sent.Conversation.Inquiry.SkuID != "SKU01" || len(sent.Conversation.Messages) != 2 {
		t.Fatalf("conversation = %s", res.Body)
	}
	if sent.Conversation.Messages[0].Kind != domain.MessageProductRef || !strings.Contains(string(sent.Conversation.Messages[0].Ref), "Prada Galleria") {
		t.Fatalf("product card = %+v", sent.Conversation.Messages[0])
	}
	id := sent.Conversation.Inquiry.ID

	// Staff see the web inquiry under the web channel with its context.
	res = do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries?channel=web", staffToken, "")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"customer_id":"user-1"`) ||
		!strings.Contains(res.Body.String(), `"channel":"web"`) || !strings.Contains(res.Body.String(), `"customer_email":"user-1@example.com"`) {
		t.Fatalf("web inbox: %d %s", res.Code, res.Body)
	}
	res = do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries?channel=telegram", staffToken, "")
	if strings.Contains(res.Body.String(), id) {
		t.Fatalf("a web inquiry leaked into the telegram list")
	}

	res = do(t, f.mux, http.MethodPost, "/api/v1/support/inquiries/"+id+"/reply", staffToken, `{"body":"다음 주 입고됩니다."}`)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"notice_status":"pending"`) {
		t.Fatalf("reply: %d %s", res.Code, res.Body)
	}

	res = do(t, f.mux, http.MethodGet, "/api/v1/support/web/conversation", customerToken, "")
	if !strings.Contains(res.Body.String(), `"unread":1`) || strings.Contains(res.Body.String(), "manager-1") {
		t.Fatalf("shopper view must count the reply unread and never name the manager: %s", res.Body)
	}
	if res := do(t, f.mux, http.MethodPost, "/api/v1/support/web/read", customerToken, ""); res.Code != http.StatusNoContent {
		t.Fatalf("read: %d", res.Code)
	}

	// Another shopper sees only their own (empty) conversation.
	res = do(t, f.mux, http.MethodGet, "/api/v1/support/web/conversation", otherToken, "")
	if strings.Contains(res.Body.String(), id) {
		t.Fatalf("another customer's conversation leaked: %s", res.Body)
	}

	res = do(t, f.mux, http.MethodPost, "/api/v1/support/web/inquiries/current/close", customerToken, "")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"inquiry":null`) {
		t.Fatalf("close: %d %s", res.Code, res.Body)
	}
}

func TestWebChatErrorCodes(t *testing.T) {
	f := newWebFixture(t)
	cases := []struct {
		name, token, body string
		status            int
		code              string
	}{
		{"empty", customerToken, `{"body":""}`, http.StatusBadRequest, "invalid_message"},
		{"unknown sku", customerToken, `{"body":"x","sku_id":"NOPE"}`, http.StatusUnprocessableEntity, "invalid_reference"},
		{"someone else's order", otherToken, `{"order_id":"ORD1"}`, http.StatusUnprocessableEntity, "invalid_reference"},
	}
	for _, tc := range cases {
		res := do(t, f.mux, http.MethodPost, "/api/v1/support/web/messages", tc.token, tc.body)
		if res.Code != tc.status || !strings.Contains(res.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatalf("%s: %d %s", tc.name, res.Code, res.Body)
		}
	}
	res := do(t, f.mux, http.MethodPost, "/api/v1/support/web/messages", customerToken, `{"order_id":"ORD1"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("own order: %d %s", res.Code, res.Body)
	}
}

func TestWebChatRefusesServiceAccountsAndAnonymous(t *testing.T) {
	f := newWebFixture(t)
	if res := do(t, f.mux, http.MethodGet, "/api/v1/support/web/conversation", "", ""); res.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", res.Code)
	}
	if res := do(t, f.mux, http.MethodGet, "/api/v1/support/web/conversation", "svc||service", ""); res.Code != http.StatusForbidden {
		t.Fatalf("service account: %d", res.Code)
	}
}

func TestWebChatUnconfiguredAnswers503(t *testing.T) {
	h := handler.New(handler.Options{
		Inbox:        service.NewInbox(memory.NewConversationRepository(), memory.NewInquiryRepository(), memory.NewMessageRepository(), &recordingBot{}, nil, nil),
		JWTValidator: webValidator{},
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	if res := do(t, mux, http.MethodGet, "/api/v1/support/web/conversation", customerToken, ""); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.Code)
	}
}

func TestStaffCannotSendCardsToTelegram(t *testing.T) {
	f := newInboxFixture(t)
	res := do(t, f.mux, http.MethodPost, "/api/v1/support/inquiries/"+f.inquiryID+"/reply",
		"manager-1|"+permissions.SupportReply, `{"body":"보세요","sku_id":"SKU01"}`)
	if res.Code != http.StatusUnprocessableEntity || !strings.Contains(res.Body.String(), "reference_on_telegram") {
		t.Fatalf("status = %d %s", res.Code, res.Body)
	}
}

func TestInboxEventsRequireSupportRead(t *testing.T) {
	f := newWebFixture(t)
	if res := do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries/events", customerToken, ""); res.Code != http.StatusForbidden {
		t.Fatalf("customer on inbox stream: %d", res.Code)
	}
}

// openStream connects to an SSE route and returns a reader past the ready
// frame.
func openStream(t *testing.T, f *webFixture, path, token string) (*bufio.Reader, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("connect: %v", err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		cancel()
		t.Fatalf("stream: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(res.Body)
	if event, _ := nextEvent(t, reader); event != "ready" {
		cancel()
		t.Fatalf("first event = %q, want ready", event)
	}
	return reader, func() { cancel(); _ = res.Body.Close() }
}

func nextEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()
	var event, data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && event != "":
			return event, data
		}
	}
}

func TestStreamsCarryIdsToTheRightPeople(t *testing.T) {
	f := newWebFixture(t)

	mine, closeMine := openStream(t, f, "/api/v1/support/web/events", customerToken)
	defer closeMine()
	inbox, closeInbox := openStream(t, f, "/api/v1/support/inquiries/events", staffToken)
	defer closeInbox()
	_, closeOther := openStream(t, f, "/api/v1/support/web/events", otherToken)
	defer closeOther()

	deadline := time.Now().Add(2 * time.Second)
	for f.hub.Subscribers() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	res := do(t, f.mux, http.MethodPost, "/api/v1/support/web/messages", customerToken, `{"body":"비밀 주소 서울시"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("send: %d %s", res.Code, res.Body)
	}

	event, data := nextEvent(t, mine)
	if event != "inquiry" && event != "message" {
		t.Fatalf("shopper stream event = %q", event)
	}
	if strings.Contains(data, "서울시") {
		t.Fatalf("a stream frame carried the message body: %s", data)
	}
	event, data = nextEvent(t, inbox)
	if (event != "inquiry" && event != "message") || !strings.Contains(data, `"channel":"web"`) {
		t.Fatalf("inbox stream = %q %s", event, data)
	}
	// user-2's stream got nothing: checked by the hub's own tests, which can
	// assert an empty channel without a read deadline on a socket.
}

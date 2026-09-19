package handler_test

import (
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
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

// stubValidator accepts a token shaped "user|perm,perm" so a test can state a
// caller's permissions without minting a real JWT.
type stubValidator struct{}

func (stubValidator) ValidateAccessToken(token string) (authjwt.Claims, error) {
	user, perms, _ := strings.Cut(token, "|")
	claims := authjwt.Claims{UserID: user}
	if perms != "" {
		claims.Permissions = strings.Split(perms, ",")
	}
	return claims, nil
}

type inboxFixture struct {
	mux       *http.ServeMux
	inquiryID string
	bot       *recordingBot
}

// recordingBot stands in for Telegram.
type recordingBot struct {
	replies []string
	err     error
}

func (b *recordingBot) Reply(_ context.Context, _ string, text string) error {
	if b.err != nil {
		return b.err
	}
	b.replies = append(b.replies, text)
	return nil
}

func (b *recordingBot) ReplyMenu(context.Context, string, string, []ports.MenuButton) error {
	return nil
}

func (b *recordingBot) EditMenu(context.Context, string, int64, string, []ports.MenuButton) error {
	return nil
}

func (b *recordingBot) AnswerCallback(context.Context, string, string) error { return nil }

func newInboxFixture(t *testing.T) *inboxFixture {
	t.Helper()
	conversations := memory.NewConversationRepository()
	inquiries := memory.NewInquiryRepository()
	messages := memory.NewMessageRepository()
	bot := &recordingBot{}

	now := time.Date(2026, 9, 17, 11, 0, 0, 0, time.UTC)
	conversation := domain.NewConversation("conv-1", "42", now)
	conversation.Username = "shopper"
	if err := conversations.Save(t.Context(), conversation); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	inquiry := domain.NewInquiry("inq-1", "conv-1", "42", domain.NodeAgent, now)
	if err := inquiries.Save(t.Context(), inquiry); err != nil {
		t.Fatalf("save inquiry: %v", err)
	}

	inbox := service.NewInbox(conversations, inquiries, messages, bot,
		func() string { return "msg-1" }, func() time.Time { return now })

	h := handler.New(handler.Options{
		Inbox:        inbox,
		Answers:      memory.NewAnswerRepository(),
		JWTValidator: stubValidator{},
		Settings:     settings.NewResponse("support"),
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return &inboxFixture{mux: mux, inquiryID: "inq-1", bot: bot}
}

func do(t *testing.T, mux *http.ServeMux, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res
}

func TestInboxRequiresAToken(t *testing.T) {
	f := newInboxFixture(t)
	// The transcript is the whole point of the authenticated surface: a
	// shopper's conversation must not be readable without one.
	if res := do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries", "", ""); res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}

func TestReadingNeedsSupportRead(t *testing.T) {
	f := newInboxFixture(t)

	if res := do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries", "someone|order.ship", ""); res.Code != http.StatusForbidden {
		t.Fatalf("an unrelated permission got %d, want 403", res.Code)
	}
	res := do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries", "agent|"+permissions.SupportRead, "")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
}

func TestReplyingNeedsSupportReply(t *testing.T) {
	f := newInboxFixture(t)
	path := "/api/v1/support/inquiries/" + f.inquiryID + "/reply"

	// Read-only staff may look but not answer.
	res := do(t, f.mux, http.MethodPost, path, "watcher|"+permissions.SupportRead, `{"body":"안녕하세요"}`)
	if res.Code != http.StatusForbidden {
		t.Fatalf("support.read alone got %d, want 403", res.Code)
	}

	res = do(t, f.mux, http.MethodPost, path, "agent|"+permissions.SupportReply, `{"body":"안녕하세요"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
	var out struct {
		Delivered bool `json:"delivered"`
		Inquiry   struct {
			AssignedTo string `json:"assigned_to"`
			Transcript []struct {
				Author string `json:"author"`
				Body   string `json:"body"`
			} `json:"transcript"`
		} `json:"inquiry"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Delivered {
		t.Fatal("a delivered reply must say so")
	}
	if out.Inquiry.AssignedTo != "agent" {
		t.Fatalf("assigned to %q, want the replying manager", out.Inquiry.AssignedTo)
	}
	if len(out.Inquiry.Transcript) != 1 || out.Inquiry.Transcript[0].Author != "agent" {
		t.Fatalf("transcript = %+v", out.Inquiry.Transcript)
	}
}

func TestEditingAnswersNeedsSupportManage(t *testing.T) {
	f := newInboxFixture(t)
	body := `{"node":"ret","body":"<b>새 안내</b>"}`

	res := do(t, f.mux, http.MethodPut, "/api/v1/support/answers", "agent|"+permissions.SupportReply, body)
	if res.Code != http.StatusForbidden {
		t.Fatalf("support.reply got %d for answer editing, want 403", res.Code)
	}
	res = do(t, f.mux, http.MethodPut, "/api/v1/support/answers", "admin|"+permissions.SupportManage, body)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
}

func TestAnswerEditsAreValidated(t *testing.T) {
	f := newInboxFixture(t)
	token := "admin|" + permissions.SupportManage

	// An unknown node would be copy no menu can ever show.
	res := do(t, f.mux, http.MethodPut, "/api/v1/support/answers", token, `{"node":"nope","body":"x"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown node got %d, want 400", res.Code)
	}
	// An empty body would kill the node: Telegram rejects an empty message.
	res = do(t, f.mux, http.MethodPut, "/api/v1/support/answers", token, `{"node":"ret","body":"  "}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("empty answer got %d, want 400", res.Code)
	}
}

func TestInquiryJSONNeverCarriesTheChatID(t *testing.T) {
	// The chat id is the shopper's Telegram identity and the console never
	// needs it to answer.
	f := newInboxFixture(t)
	res := do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries/"+f.inquiryID, "agent|"+permissions.SupportRead, "")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	if strings.Contains(res.Body.String(), `"chat_id"`) {
		t.Fatalf("inquiry JSON exposes the chat id: %s", res.Body.String())
	}
}

func TestUnknownInquiryIs404(t *testing.T) {
	f := newInboxFixture(t)
	res := do(t, f.mux, http.MethodGet, "/api/v1/support/inquiries/nope", "agent|"+permissions.SupportRead, "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.Code)
	}
}

func TestInboxIs503WithoutAValidator(t *testing.T) {
	// No auth configured must mean no access, not open access.
	h := handler.New(handler.Options{Settings: settings.NewResponse("support")})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	if res := do(t, mux, http.MethodGet, "/api/v1/support/inquiries", "anything", ""); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.Code)
	}
}

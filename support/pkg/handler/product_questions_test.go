package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/elug3/dupli1/support/pkg/handler"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	"github.com/elug3/dupli1/support/pkg/service"
)

func newQuestionMux(t *testing.T, withValidator bool) *http.ServeMux {
	t.Helper()
	n := 0
	questions := service.NewProductQuestions(service.ProductQuestionsDeps{
		Repo:     memory.NewProductQuestionRepository(),
		Products: webProducts{},
		NewID: func() string {
			n++
			return fmt.Sprintf("Q%d", n)
		},
		Now: func() time.Time { return time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC) },
	})
	opts := handler.Options{
		Answers:   memory.NewAnswerRepository(),
		Settings:  settings.NewResponse("support"),
		Questions: questions,
	}
	if withValidator {
		opts.JWTValidator = webValidator{}
	}
	mux := http.NewServeMux()
	handler.New(opts).RegisterRoutes(mux)
	return mux
}

type questionList struct {
	Questions []struct {
		ID            string `json:"id"`
		Author        string `json:"author"`
		Body          string `json:"body"`
		Answer        string `json:"answer"`
		Redacted      bool   `json:"redacted"`
		Mine          bool   `json:"mine"`
		Editable      bool   `json:"editable"`
		CustomerEmail string `json:"customer_email"`
	} `json:"questions"`
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
}

func decodeList(t *testing.T, body []byte) questionList {
	t.Helper()
	var out questionList
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return out
}

func TestProductQuestionsRoundTrip(t *testing.T) {
	mux := newQuestionMux(t, true)
	path := "/api/v1/support/products/P01/questions"

	if res := do(t, mux, http.MethodPost, path, "", `{"sku_id":"SKU01","type":"size","body":"M?"}`); res.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous ask: %d", res.Code)
	}
	if res := do(t, mux, http.MethodPost, path, "svc||service", `{"sku_id":"SKU01","type":"size","body":"M?"}`); res.Code != http.StatusForbidden {
		t.Fatalf("service account ask: %d", res.Code)
	}
	if res := do(t, mux, http.MethodPost, path, customerToken, `{"sku_id":"SKU01","type":"delivery","body":"언제 와요?"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("delivery type: %d %s", res.Code, res.Body)
	}
	if res := do(t, mux, http.MethodPost, "/api/v1/support/products/OTHER/questions", customerToken, `{"sku_id":"SKU01","type":"size","body":"M?"}`); res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("sku of another product: %d %s", res.Code, res.Body)
	}

	res := do(t, mux, http.MethodPost, path, customerToken,
		`{"sku_id":"SKU01","type":"size","body":"평소 L인데 M?","secret":true,"fit":{"height_cm":178,"weight_kg":70,"usual_size":"L"}}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("ask: %d %s", res.Code, res.Body)
	}
	var asked struct {
		ID       string `json:"id"`
		Editable bool   `json:"editable"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &asked)
	if !asked.Editable {
		t.Fatalf("own new question not editable: %s", res.Body)
	}

	// Anyone reads the list; a stranger sees the secret one only as a row.
	res = do(t, mux, http.MethodGet, path, "", "")
	if res.Code != http.StatusOK {
		t.Fatalf("anonymous list: %d %s", res.Code, res.Body)
	}
	list := decodeList(t, res.Body.Bytes())
	if list.Total != 1 || list.Counts["size"] != 1 || !list.Questions[0].Redacted || list.Questions[0].Body != "" || list.Questions[0].Author != "us****" {
		t.Fatalf("anonymous list = %s", res.Body)
	}
	list = decodeList(t, do(t, mux, http.MethodGet, path, customerToken, "").Body.Bytes())
	if list.Questions[0].Redacted || !list.Questions[0].Mine || list.Questions[0].CustomerEmail != "" {
		t.Fatalf("author list = %+v", list.Questions[0])
	}

	// Staff only, and permission-checked.
	staffPath := "/api/v1/support/product-questions"
	if res := do(t, mux, http.MethodGet, staffPath, customerToken, ""); res.Code != http.StatusForbidden {
		t.Fatalf("customer read queue: %d", res.Code)
	}
	readOnly := "manager-2|" + permissions.SupportRead + "|manager"
	if res := do(t, mux, http.MethodPost, staffPath+"/"+asked.ID+"/answer", readOnly, `{"answer":"M"}`); res.Code != http.StatusForbidden {
		t.Fatalf("read-only answer: %d", res.Code)
	}
	queue := decodeList(t, do(t, mux, http.MethodGet, staffPath+"?queue=waiting", staffToken, "").Body.Bytes())
	if len(queue.Questions) != 1 || queue.Questions[0].CustomerEmail != "user-1@example.com" {
		t.Fatalf("queue = %+v", queue)
	}
	if res := do(t, mux, http.MethodPost, staffPath+"/"+asked.ID+"/answer", staffToken, `{"answer":"M을 권해드려요."}`); res.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", res.Code, res.Body)
	}

	// Answered: locked for its author.
	mine := "/api/v1/support/me/product-questions/" + asked.ID
	if res := do(t, mux, http.MethodPatch, mine, customerToken, `{"type":"size","body":"바꿀게요"}`); res.Code != http.StatusConflict {
		t.Fatalf("edit answered: %d %s", res.Code, res.Body)
	}
	if res := do(t, mux, http.MethodDelete, mine, otherToken, ""); res.Code != http.StatusNotFound {
		t.Fatalf("stranger withdraw: %d", res.Code)
	}
	list = decodeList(t, do(t, mux, http.MethodGet, "/api/v1/support/me/product-questions", customerToken, "").Body.Bytes())
	if len(list.Questions) != 1 || list.Questions[0].Answer == "" || list.Questions[0].Editable {
		t.Fatalf("mine = %+v", list.Questions)
	}

	if res := do(t, mux, http.MethodPost, staffPath+"/"+asked.ID+"/hide", staffToken, `{}`); res.Code != http.StatusOK {
		t.Fatalf("hide: %d %s", res.Code, res.Body)
	}
	if list := decodeList(t, do(t, mux, http.MethodGet, path, "", "").Body.Bytes()); list.Total != 0 {
		t.Fatalf("hidden question still listed: %+v", list)
	}
}

func TestProductQuestionListWithoutAuthConfigured(t *testing.T) {
	mux := newQuestionMux(t, false)
	res := do(t, mux, http.MethodGet, "/api/v1/support/products/P01/questions", customerToken, "")
	if res.Code != http.StatusOK {
		t.Fatalf("public list without a validator: %d %s", res.Code, res.Body)
	}
	if res := do(t, mux, http.MethodPost, "/api/v1/support/products/P01/questions", customerToken, `{}`); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("ask without a validator: %d", res.Code)
	}
}

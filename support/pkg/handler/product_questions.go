package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/authmiddleware"
	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

// maxQuestionBody bounds a question or answer request. 2000 characters of
// Hangul is 6 KB.
const maxQuestionBody = 32 << 10

// questionJSON is a product question on the wire. The staff-only fields are
// empty (and omitted) for everyone else: viewOf blanks them in the service.
type questionJSON struct {
	ID           string      `json:"id"`
	ProductID    string      `json:"product_id"`
	SkuID        string      `json:"sku_id,omitempty"`
	VariantLabel string      `json:"variant_label,omitempty"`
	ProductName  string      `json:"product_name,omitempty"`
	Author       string      `json:"author"`
	Type         string      `json:"type"`
	Body         string      `json:"body"`
	Fit          *domain.Fit `json:"fit,omitempty"`
	Secret       bool        `json:"secret"`
	Status       string      `json:"status"`
	Answer       string      `json:"answer,omitempty"`
	AnsweredAt   *time.Time  `json:"answered_at,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	// Redacted: a secret question someone else asked; body and answer are
	// empty and the storefront shows "비밀글입니다".
	Redacted bool `json:"redacted"`
	Mine     bool `json:"mine"`
	// Editable: the viewer may still edit or withdraw it.
	Editable bool `json:"editable"`

	CustomerID    string     `json:"customer_id,omitempty"`
	CustomerEmail string     `json:"customer_email,omitempty"`
	AnsweredBy    string     `json:"answered_by,omitempty"`
	Hidden        bool       `json:"hidden,omitempty"`
	HiddenBy      string     `json:"hidden_by,omitempty"`
	HiddenAt      *time.Time `json:"hidden_at,omitempty"`
}

func questionToJSON(view service.QuestionView) questionJSON {
	return questionJSON{
		ID:            view.ID,
		ProductID:     view.ProductID,
		SkuID:         view.SkuID,
		VariantLabel:  view.VariantLabel,
		ProductName:   view.ProductName,
		Author:        view.AuthorMask,
		Type:          view.Type,
		Body:          view.Body,
		Fit:           view.Fit,
		Secret:        view.Secret,
		Status:        view.Status,
		Answer:        view.Answer,
		AnsweredAt:    view.AnsweredAt,
		CreatedAt:     view.CreatedAt,
		UpdatedAt:     view.UpdatedAt,
		Redacted:      view.Redacted,
		Mine:          view.Mine,
		Editable:      view.Mine && !view.IsAnswered(),
		CustomerID:    view.CustomerID,
		CustomerEmail: view.CustomerEmail,
		AnsweredBy:    view.AnsweredBy,
		Hidden:        view.Hidden,
		HiddenBy:      view.HiddenBy,
		HiddenAt:      view.HiddenAt,
	}
}

func questionsToJSON(views []service.QuestionView) []questionJSON {
	out := make([]questionJSON, 0, len(views))
	for _, view := range views {
		out = append(out, questionToJSON(view))
	}
	return out
}

type questionRequest struct {
	SkuID  string      `json:"sku_id"`
	Type   string      `json:"type"`
	Body   string      `json:"body"`
	Secret bool        `json:"secret"`
	Fit    *domain.Fit `json:"fit"`
}

func (h *Handler) registerProductQuestionRoutes(mux *http.ServeMux) {
	// Anyone reads a product's questions; signing in only adds "mine" and the
	// words of one's own secret questions.
	mux.HandleFunc("GET /api/v1/support/products/{productID}/questions", h.optionalAuth(h.requireQuestions(h.listProductQuestions)))
	mux.HandleFunc("POST /api/v1/support/products/{productID}/questions", h.requireAuth(h.requireQuestions(h.askProductQuestion)))

	// The shopper's own questions. No product in the path: the id is checked
	// against the caller, so it cannot point at someone else's.
	mux.HandleFunc("GET /api/v1/support/me/product-questions", h.requireAuth(h.requireQuestions(h.myProductQuestions)))
	mux.HandleFunc("PATCH /api/v1/support/me/product-questions/{id}", h.requireAuth(h.requireQuestions(h.editProductQuestion)))
	mux.HandleFunc("DELETE /api/v1/support/me/product-questions/{id}", h.requireAuth(h.requireQuestions(h.withdrawProductQuestion)))

	// The console.
	mux.HandleFunc("GET /api/v1/support/product-questions", h.requireAuth(h.requireQuestions(h.questionQueue)))
	mux.HandleFunc("GET /api/v1/support/product-questions/{id}", h.requireAuth(h.requireQuestions(h.questionDetail)))
	mux.HandleFunc("POST /api/v1/support/product-questions/{id}/answer", h.requireAuth(h.requireQuestions(h.answerProductQuestion)))
	mux.HandleFunc("POST /api/v1/support/product-questions/{id}/hide", h.requireAuth(h.requireQuestions(h.hideProductQuestion)))
}

func (h *Handler) requireQuestions(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.questions == nil {
			respondCode(w, http.StatusServiceUnavailable, "questions_unavailable", "product questions not configured")
			return
		}
		next(w, r)
	}
}

// optionalAuth reads a token when one is sent. Without a validator the public
// list is still served, anonymously: it holds nothing a token would unlock
// except the caller's own secret questions.
func (h *Handler) optionalAuth(next http.HandlerFunc) http.HandlerFunc {
	if h.jwtValidator == nil {
		return func(w http.ResponseWriter, r *http.Request) {
			r.Header.Del("Authorization")
			next(w, r)
		}
	}
	return authmiddleware.OptionalAuth(h.jwtValidator, respondError)(next)
}

func (h *Handler) listProductQuestions(w http.ResponseWriter, r *http.Request) {
	viewerID := ""
	if customer, ok := customerFrom(r); ok {
		viewerID = customer.ID
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	result, err := h.questions.List(r.Context(), r.PathValue("productID"), viewerID, service.ListQuery{
		Type:         q.Get("type"),
		AnsweredOnly: q.Get("answered") == "true",
		MineOnly:     q.Get("mine") == "true",
		Page:         page,
	})
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"questions": questionsToJSON(result.Items),
		"total":     result.Total,
		"counts":    result.Counts,
		"page":      result.Page,
		"has_more":  result.More,
	})
}

func (h *Handler) askProductQuestion(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFrom(r)
	if !ok {
		respondCode(w, http.StatusForbidden, "customer_required", "a customer account is required")
		return
	}
	var req questionRequest
	if !decodeQuestionRequest(w, r, &req) {
		return
	}
	question, err := h.questions.Ask(r.Context(), customer, service.AskInput{
		ProductID: r.PathValue("productID"),
		SkuID:     req.SkuID,
		Type:      req.Type,
		Body:      req.Body,
		Secret:    req.Secret,
		Fit:       req.Fit,
	})
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, questionToJSON(service.QuestionView{ProductQuestion: *question, Mine: true}))
}

func (h *Handler) myProductQuestions(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFrom(r)
	if !ok {
		respondCode(w, http.StatusForbidden, "customer_required", "a customer account is required")
		return
	}
	views, err := h.questions.Mine(r.Context(), customer)
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"questions": questionsToJSON(views)})
}

func (h *Handler) editProductQuestion(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFrom(r)
	if !ok {
		respondCode(w, http.StatusForbidden, "customer_required", "a customer account is required")
		return
	}
	var req questionRequest
	if !decodeQuestionRequest(w, r, &req) {
		return
	}
	view, err := h.questions.Edit(r.Context(), customer, r.PathValue("id"), service.AskInput{
		Type: req.Type, Body: req.Body, Secret: req.Secret, Fit: req.Fit,
	})
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, questionToJSON(*view))
}

func (h *Handler) withdrawProductQuestion(w http.ResponseWriter, r *http.Request) {
	customer, ok := customerFrom(r)
	if !ok {
		respondCode(w, http.StatusForbidden, "customer_required", "a customer account is required")
		return
	}
	if err := h.questions.Withdraw(r.Context(), customer, r.PathValue("id")); err != nil {
		h.respondQuestionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) questionQueue(w http.ResponseWriter, r *http.Request) {
	if !staffMay(w, r, canRead) {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	views, err := h.questions.Queue(r.Context(), ports.ProductQuestionFilter{
		Queue:     q.Get("queue"),
		Type:      q.Get("type"),
		ProductID: q.Get("product_id"),
		Limit:     limit,
	})
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"questions": questionsToJSON(views)})
}

func (h *Handler) questionDetail(w http.ResponseWriter, r *http.Request) {
	if !staffMay(w, r, canRead) {
		return
	}
	view, err := h.questions.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, questionToJSON(*view))
}

func (h *Handler) answerProductQuestion(w http.ResponseWriter, r *http.Request) {
	if !staffMay(w, r, canReply) {
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	var req struct {
		Answer string `json:"answer"`
	}
	if !decodeQuestionRequest(w, r, &req) {
		return
	}
	view, err := h.questions.Answer(r.Context(), managerID(claims), r.PathValue("id"), req.Answer)
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, questionToJSON(*view))
}

func (h *Handler) hideProductQuestion(w http.ResponseWriter, r *http.Request) {
	if !staffMay(w, r, canReply) {
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	var req struct {
		Hidden *bool `json:"hidden"`
	}
	if !decodeQuestionRequest(w, r, &req) {
		return
	}
	hidden := req.Hidden == nil || *req.Hidden
	view, err := h.questions.SetHidden(r.Context(), managerID(claims), r.PathValue("id"), hidden)
	if err != nil {
		h.respondQuestionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, questionToJSON(*view))
}

func staffMay(w http.ResponseWriter, r *http.Request, allowed func(authjwt.Claims) bool) bool {
	claims, ok := authjwt.FromContext(r.Context())
	if !ok || !allowed(claims) {
		respondError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func decodeQuestionRequest(w http.ResponseWriter, r *http.Request, into any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxQuestionBody+1))
	if err != nil || len(body) > maxQuestionBody {
		respondCode(w, http.StatusBadRequest, "invalid_question", "invalid body")
		return false
	}
	if err := json.Unmarshal(body, into); err != nil {
		respondCode(w, http.StatusBadRequest, "invalid_question", "invalid JSON")
		return false
	}
	return true
}

// respondQuestionError maps service errors to the stable codes the storefront
// and console write their copy against.
func (h *Handler) respondQuestionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidQuestion):
		respondCode(w, http.StatusBadRequest, "invalid_question", err.Error())
	case errors.Is(err, service.ErrInvalidAnswer):
		respondCode(w, http.StatusBadRequest, "invalid_answer", err.Error())
	case errors.Is(err, service.ErrInvalidReference):
		respondCode(w, http.StatusUnprocessableEntity, "invalid_reference", err.Error())
	case errors.Is(err, service.ErrReferencesUnavailable):
		respondCode(w, http.StatusServiceUnavailable, "reference_unavailable", err.Error())
	case errors.Is(err, service.ErrRateLimited):
		w.Header().Set("Retry-After", "60")
		respondCode(w, http.StatusTooManyRequests, "rate_limited", err.Error())
	case errors.Is(err, service.ErrQuestionNotFound):
		respondCode(w, http.StatusNotFound, "question_not_found", err.Error())
	case errors.Is(err, service.ErrQuestionLocked):
		respondCode(w, http.StatusConflict, "question_answered", err.Error())
	case errors.Is(err, service.ErrCustomerRequired):
		respondCode(w, http.StatusForbidden, "customer_required", err.Error())
	default:
		log.Printf("product questions: %v", err)
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

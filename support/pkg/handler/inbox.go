package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

// inquiryJSON is the wire shape of an inquiry in the console.
//
// The chat id is deliberately absent: it is the shopper's Telegram identity,
// the console never needs it to answer, and an id that never leaves the service
// cannot leak from the admin UI.
type inquiryJSON struct {
	ID           string        `json:"id"`
	Topic        string        `json:"topic"`
	Status       string        `json:"status"`
	AssignedTo   string        `json:"assigned_to,omitempty"`
	Language     string        `json:"language,omitempty"`
	Username     string        `json:"username,omitempty"`
	EntryContext string        `json:"entry_context,omitempty"`
	LastMessage  string        `json:"last_message,omitempty"`
	OpenedAt     time.Time     `json:"opened_at"`
	ClosedAt     *time.Time    `json:"closed_at,omitempty"`
	Transcript   []messageJSON `json:"transcript,omitempty"`
}

type messageJSON struct {
	ID            string    `json:"id"`
	Direction     string    `json:"direction"`
	Author        string    `json:"author,omitempty"`
	Body          string    `json:"body"`
	Delivery      string    `json:"delivery,omitempty"`
	DeliveryError string    `json:"delivery_error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func inquiryToJSON(view service.InquiryView, withTranscript bool) inquiryJSON {
	out := inquiryJSON{
		ID:           view.ID,
		Topic:        view.Topic,
		Status:       view.Status,
		AssignedTo:   view.AssignedTo,
		Language:     view.Language,
		Username:     view.Username,
		EntryContext: view.EntryPayload,
		LastMessage:  view.LastMessage,
		OpenedAt:     view.OpenedAt,
		ClosedAt:     view.ClosedAt,
	}
	if withTranscript {
		out.Transcript = make([]messageJSON, 0, len(view.Transcript))
		for _, message := range view.Transcript {
			out.Transcript = append(out.Transcript, messageJSON{
				ID:            message.ID,
				Direction:     message.Direction,
				Author:        message.Author,
				Body:          message.Body,
				Delivery:      message.Delivery,
				DeliveryError: message.DeliveryError,
				CreatedAt:     message.CreatedAt,
			})
		}
	}
	return out
}

func (h *Handler) inquiries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	if !canRead(claims) {
		respondError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	filter := ports.InquiryFilter{
		Status:     strings.TrimSpace(r.URL.Query().Get("status")),
		AssignedTo: strings.TrimSpace(r.URL.Query().Get("assigned_to")),
	}
	// ?queue=waiting is the 대기 list: open and claimed by nobody.
	if strings.TrimSpace(r.URL.Query().Get("queue")) == "waiting" {
		filter.Unassigned = true
	}
	// ?assigned_to=me resolves to the caller, so the console never has to know
	// its own user id to ask for its own work.
	if filter.AssignedTo == "me" {
		filter.AssignedTo = managerID(claims)
	}

	views, err := h.inbox.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list inquiries")
		return
	}
	out := make([]inquiryJSON, 0, len(views))
	for _, view := range views {
		out = append(out, inquiryToJSON(view, false))
	}
	respondJSON(w, http.StatusOK, map[string]any{"inquiries": out})
}

// inquiryAction serves /inquiries/{id} and its sub-routes.
func (h *Handler) inquiryAction(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/support/inquiries/")
	id, action, _ := strings.Cut(rest, "/")
	if strings.TrimSpace(id) == "" {
		respondError(w, http.StatusNotFound, "inquiry not found")
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		if !canRead(claims) {
			respondError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		view, err := h.inbox.Get(r.Context(), id)
		h.respondInquiry(w, view, err, true)

	case action == "assign" && r.Method == http.MethodPost:
		if !canReply(claims) {
			respondError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		view, err := h.inbox.Claim(r.Context(), id, managerID(claims))
		h.respondInquiry(w, view, err, true)

	case action == "reply" && r.Method == http.MethodPost:
		if !canReply(claims) {
			respondError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		h.reply(w, r, id, managerID(claims))

	case action == "close" && r.Method == http.MethodPost:
		if !canReply(claims) {
			respondError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		view, err := h.inbox.Close(r.Context(), id, managerID(claims))
		h.respondInquiry(w, view, err, true)

	default:
		respondError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, id, manager string) {
	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}

	view, err := h.inbox.Reply(r.Context(), id, manager, req.Body)
	// An undeliverable reply is stored and shown as 미전송 rather than reported
	// as a failure: the manager did their part, and the record must say the
	// shopper never got it.
	if errors.Is(err, service.ErrUndeliverable) {
		respondJSON(w, http.StatusOK, map[string]any{
			"inquiry":   inquiryToJSON(*view, true),
			"delivered": false,
			"error":     "undeliverable",
		})
		return
	}
	if err != nil {
		h.respondInquiry(w, view, err, true)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"inquiry":   inquiryToJSON(*view, true),
		"delivered": true,
	})
}

func (h *Handler) respondInquiry(w http.ResponseWriter, view *service.InquiryView, err error, withTranscript bool) {
	switch {
	case errors.Is(err, service.ErrInquiryNotFound):
		respondError(w, http.StatusNotFound, "inquiry not found")
	case err != nil:
		respondError(w, http.StatusInternalServerError, "could not load inquiry")
	default:
		respondJSON(w, http.StatusOK, map[string]any{"inquiry": inquiryToJSON(*view, withTranscript)})
	}
}

// answersHandler serves the canned copy staff edit without a deploy.
func (h *Handler) answersHandler(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	if !canManage(claims) {
		respondError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	switch r.Method {
	case http.MethodGet:
		answers, err := h.answers.All(r.Context(), domain.DefaultLanguage)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not load answers")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"answers": answers})
	case http.MethodPut:
		var req struct {
			Node string `json:"node"`
			Body string `json:"body"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if !domain.IsKnownNode(req.Node) {
			respondError(w, http.StatusBadRequest, "unknown menu node")
			return
		}
		if strings.TrimSpace(req.Body) == "" {
			// Telegram rejects an empty message, so an empty answer would kill
			// the node rather than clear it.
			respondError(w, http.StatusBadRequest, "answer body is required")
			return
		}
		err := h.answers.Put(r.Context(), req.Node, domain.DefaultLanguage, req.Body, managerID(claims))
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not save answer")
			return
		}
		respondJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func canRead(claims authjwt.Claims) bool {
	return claims.HasPermission(
		permissions.SupportRead, permissions.SupportReply, permissions.SupportManage,
		permissions.All, permissions.AdminAll,
	)
}

func canReply(claims authjwt.Claims) bool {
	return claims.HasPermission(
		permissions.SupportReply, permissions.SupportManage,
		permissions.All, permissions.AdminAll,
	)
}

func canManage(claims authjwt.Claims) bool {
	return claims.HasPermission(permissions.SupportManage, permissions.All, permissions.AdminAll)
}

// managerID is who the audit trail records. The console shows it against every
// reply, which is what Telegram alone could never tell you.
func managerID(claims authjwt.Claims) string {
	return claims.UserID
}

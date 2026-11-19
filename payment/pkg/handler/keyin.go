package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

// maxCardBody bounds the key-in request; the real body is ~200 bytes.
const maxCardBody = 4 << 10

// cardRequest is POST /api/v1/payments/{id}/card. It is decoded here, handed
// to the service and dropped: it is never logged or echoed, and decode
// errors are answered with a fixed message so no part of it is reflected.
type cardRequest struct {
	CardNumber   string `json:"card_number"`
	ExpMonth     string `json:"exp_month"`
	ExpYear      string `json:"exp_year"`
	CardPassword string `json:"card_password"`
	HolderID     string `json:"holder_id"`
	Installment  string `json:"installment"`
}

// payByCard charges the card the shopper typed (NANO 수기결제). Only the
// shopper who owns the payment may call it; staff have no key-in path.
func (h *Handler) payByCard(w http.ResponseWriter, r *http.Request, paymentID string) {
	w.Header().Set("Cache-Control", "no-store")
	claims, _ := authjwt.FromContext(r.Context())
	var req cardRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxCardBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "card details must be a JSON object")
		return
	}
	bearer := ""
	if auth := r.Header.Get("Authorization"); len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		bearer = auth[7:]
	}
	payment, err := h.svc.PayByCard(r.Context(), service.PayByCardInput{
		PaymentID:   paymentID,
		CustomerID:  claims.UserID,
		BearerToken: bearer,
		Card: domain.CardDetails{
			Number:      req.CardNumber,
			ExpYY:       req.ExpYear,
			ExpMM:       req.ExpMonth,
			Password2:   req.CardPassword,
			HolderID:    req.HolderID,
			Installment: req.Installment,
		},
	})
	req = cardRequest{}
	if err != nil {
		respondCardError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, payment)
}

// respondCardError gives the storefront a reason it can word for the shopper.
// Every key-in failure carries one; anything else falls through to the usual
// service error mapping.
func respondCardError(w http.ResponseWriter, err error) {
	var field *domain.CardFieldError
	var declined *service.CardDeclinedError
	switch {
	case errors.As(err, &field):
		respondJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": "invalid card details", "code": http.StatusUnprocessableEntity,
			"reason": "invalid_card", "field": field.Field,
		})
	case errors.As(err, &declined):
		respondJSON(w, http.StatusPaymentRequired, map[string]any{
			"error": "card declined", "code": http.StatusPaymentRequired,
			"reason":             "card_declined",
			"message":            declined.Message,
			"result_code":        declined.Code,
			"attempts_remaining": declined.AttemptsRemaining,
		})
	case errors.Is(err, domain.ErrCardAttemptsExhausted):
		respondJSON(w, http.StatusConflict, map[string]any{
			"error": err.Error(), "code": http.StatusConflict, "reason": "card_attempts_exhausted",
		})
	case errors.Is(err, domain.ErrCardApprovalInProgress):
		respondJSON(w, http.StatusConflict, map[string]any{
			"error": err.Error(), "code": http.StatusConflict, "reason": "card_approval_in_progress",
		})
	case errors.Is(err, domain.ErrCardApprovalHeld):
		// The card may have been charged: the storefront must not offer
		// another try, only say we will check.
		respondJSON(w, http.StatusConflict, map[string]any{
			"error": err.Error(), "code": http.StatusConflict, "reason": "card_approval_unconfirmed",
		})
	default:
		respondServiceError(w, err)
	}
}

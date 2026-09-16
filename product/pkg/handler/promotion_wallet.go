package handler

import (
	"encoding/json"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

// PromotionWallet lists the signed-in customer's promotional codes, each judged
// against the cart they are looking at.
//
// ABAC, not a permission: a customer reads their own wallet because the JWT
// subject is the owner. The customer id is taken from the token and never from
// the body, so one account cannot read another's.
//
// Ineligible entries come back with a reason rather than being hidden — a
// customer who knows they have a code is owed an explanation, not silence.
func (h *Handler) PromotionWallet(w http.ResponseWriter, r *http.Request) {
	claims, ok := authjwt.FromContext(r.Context())
	if !ok || claims.UserID == "" {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// The cart is optional: with no body the wallet still lists what the
	// customer holds, just with nothing to judge it against.
	var body struct {
		ShippingFeeWon int64                   `json:"shipping_fee_won"`
		Lines          []domain.EvaluationLine `json:"lines"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	entries, err := h.promotionSvc.Wallet(r.Context(), claims.UserID, domain.EvaluationContext{
		ShippingFeeWon: body.ShippingFeeWon,
		Lines:          body.Lines,
	})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, map[string]any{
		"total":   len(entries),
		"results": entries,
	})
}

// IssuePromotion grants one account the right to use a single-user code.
//
// Managers use this for goodwill and for codes the automatic issuer missed.
// It is idempotent on the trigger key, so re-running a bulk issue does not
// hand anyone a second entitlement.
func (h *Handler) IssuePromotion(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		h.respondError(w, http.StatusBadRequest, "missing promotion code")
		return
	}
	var body struct {
		CustomerID string `json:"customer_id"`
		TriggerKey string `json:"trigger_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	issuedBy := ""
	if claims, ok := authjwt.FromContext(r.Context()); ok {
		issuedBy = claims.UserID
	}
	// Without a caller-supplied key, one manager issue per customer per code.
	triggerKey := body.TriggerKey
	if triggerKey == "" {
		triggerKey = "manager:" + body.CustomerID
	}

	entitlement, err := h.promotionSvc.Issue(r.Context(), code, body.CustomerID, "issue", triggerKey, issuedBy)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, entitlement)
}

// RevokePromotionEntitlement withdraws an entitlement issued by mistake. It
// never rewrites an order that already used the code.
func (h *Handler) RevokePromotionEntitlement(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing entitlement id")
		return
	}
	if err := h.promotionSvc.Revoke(r.Context(), id); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

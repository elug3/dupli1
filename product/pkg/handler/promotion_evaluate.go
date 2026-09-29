package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

// EvaluatePromotion judges a code against a specific checkout and returns the
// discount it earns.
//
// This is what order calls at apply and again at checkout complete. It
// replaces the old redeem-then-trust-the-caller shape, where product answered
// only "does this code exist" and the discount was computed elsewhere against
// whatever cart the caller claimed.
//
// Rejections carry a machine-readable reason so customer copy stays in the
// frontends. A `200` with `ok: false` is the normal way to say "not eligible" —
// the request itself succeeded.
func (h *Handler) EvaluatePromotion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code           string                  `json:"code"`
		CustomerID     string                  `json:"customer_id"`
		ShippingFeeWon int64                   `json:"shipping_fee_won"`
		Lines          []domain.EvaluationLine `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Code == "" {
		h.respondError(w, http.StatusBadRequest, "code is required")
		return
	}

	result := h.promotionSvc.Evaluate(r.Context(), body.Code, domain.EvaluationContext{
		CustomerID:     body.CustomerID,
		ShippingFeeWon: body.ShippingFeeWon,
		Lines:          body.Lines,
	})
	h.respondJSON(w, http.StatusOK, result)
}

// EvaluateTier returns the automatic tier discount (apply_mode auto) the
// customer earns on this cart, the best one when they hold several. ok false
// with no reason means no tier applies.
//
// Order calls it when showing a checkout session and again at complete. It is
// internal because the customer id comes from the body: public, it would tell
// anyone which accounts are VIP.
func (h *Handler) EvaluateTier(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CustomerID     string                  `json:"customer_id"`
		ShippingFeeWon int64                   `json:"shipping_fee_won"`
		Lines          []domain.EvaluationLine `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.CustomerID == "" {
		h.respondError(w, http.StatusBadRequest, "customer_id is required")
		return
	}
	result, err := h.promotionSvc.EvaluateTier(r.Context(), domain.EvaluationContext{
		CustomerID:     body.CustomerID,
		ShippingFeeWon: body.ShippingFeeWon,
		Lines:          body.Lines,
	})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, result)
}

// MyTier is EvaluateTier for the signed-in customer, taken from the token,
// never the body — so one account cannot learn another's tier. The storefront
// calls it to show a member's discount before the order exists.
func (h *Handler) MyTier(w http.ResponseWriter, r *http.Request) {
	claims, ok := authjwt.FromContext(r.Context())
	if !ok || claims.UserID == "" {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		ShippingFeeWon int64                   `json:"shipping_fee_won"`
		Lines          []domain.EvaluationLine `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.promotionSvc.EvaluateTier(r.Context(), domain.EvaluationContext{
		CustomerID:     claims.UserID,
		ShippingFeeWon: body.ShippingFeeWon,
		Lines:          body.Lines,
	})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, result)
}

// ReservePromotion records a pending use against an order at checkout
// complete. It re-evaluates first, so a reservation cannot be created for a
// cart that does not earn the discount.
func (h *Handler) ReservePromotion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code           string                  `json:"code"`
		OrderID        string                  `json:"order_id"`
		CustomerID     string                  `json:"customer_id"`
		ShippingFeeWon int64                   `json:"shipping_fee_won"`
		Lines          []domain.EvaluationLine `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Code == "" || body.OrderID == "" {
		h.respondError(w, http.StatusBadRequest, "code and order_id are required")
		return
	}

	redemption, result, err := h.promotionSvc.Reserve(r.Context(), body.Code, body.OrderID, domain.EvaluationContext{
		CustomerID:     body.CustomerID,
		ShippingFeeWon: body.ShippingFeeWon,
		Lines:          body.Lines,
	})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, map[string]any{
		"result":     result,
		"redemption": redemption,
	})
}

// ConsumePromotion marks an order's reservation paid. Idempotent: payment
// events can be redelivered.
func (h *Handler) ConsumePromotion(w http.ResponseWriter, r *http.Request) {
	h.redemptionTransition(w, r, h.promotionSvc.Consume)
}

// ReleasePromotion hands a use back, for a cancel before shipment. The caller
// decides which cancels are releasable — an order cancelled after it shipped
// keeps its redemption consumed.
func (h *Handler) ReleasePromotion(w http.ResponseWriter, r *http.Request) {
	h.redemptionTransition(w, r, h.promotionSvc.Release)
}

func (h *Handler) redemptionTransition(w http.ResponseWriter, r *http.Request, apply func(context.Context, string) error) {
	var body struct {
		OrderID string `json:"order_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.OrderID == "" {
		h.respondError(w, http.StatusBadRequest, "order_id is required")
		return
	}
	if err := apply(r.Context(), body.OrderID); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

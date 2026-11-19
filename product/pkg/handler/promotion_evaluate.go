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

// noTier is the whole answer of the retired tier routes: no tier applies.
var noTier = map[string]bool{"ok": false}

// EvaluateTier used to return a customer's automatic tier discount (apply_mode
// auto) for order. Tiers were retired on 2026-11-18 for the points multiplier;
// the route stays one release so an order replica deployed before this one
// still completes checkouts, and always answers that no tier applies.
// Remove it, with MyTier and both routes, once order no longer calls it.
func (h *Handler) EvaluateTier(w http.ResponseWriter, _ *http.Request) {
	h.respondJSON(w, http.StatusOK, noTier)
}

// MyTier is EvaluateTier for the storefront, kept as long for an old page.
func (h *Handler) MyTier(w http.ResponseWriter, r *http.Request) {
	if claims, ok := authjwt.FromContext(r.Context()); !ok || claims.UserID == "" {
		h.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.respondJSON(w, http.StatusOK, noTier)
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

	res, err := h.promotionSvc.ReserveForOrder(r.Context(), body.Code, body.OrderID, domain.EvaluationContext{
		CustomerID:     body.CustomerID,
		ShippingFeeWon: body.ShippingFeeWon,
		Lines:          body.Lines,
	})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	// referral is only on this route, which only dupli1-order reaches (the
	// internal listener, service account): order snapshots it onto the order
	// for the commission report. The public evaluate route never carries it.
	out := map[string]any{
		"result":     res.Result,
		"redemption": res.Redemption,
	}
	if res.Referral != nil {
		out["referral"] = res.Referral
	}
	h.respondJSON(w, http.StatusOK, out)
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

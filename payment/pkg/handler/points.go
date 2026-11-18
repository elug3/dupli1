package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/serviceaccount"
)

// WithPoints enables the points routes (docs/payment-points.md).
func (h *Handler) WithPoints(points *service.Points) *Handler {
	h.points = points
	return h
}

// pointsRoutes serves /api/v1/payments/points/…:
//
//	GET  me                          the signed-in customer's balance, tier, history
//	GET  me/entries?cursor=          more of that history
//	GET  customers/{id}              any customer's (payment.points.read)
//	GET  customers/{id}/entries      more of it
//	POST customers/{id}/adjust       grant or take away (payment.points.manage)
//	POST reserve|consume|release     checkout holds (dupli1-order only, internal listener)
func (h *Handler) pointsRoutes(w http.ResponseWriter, r *http.Request, rest []string) {
	if h.points == nil {
		respondError(w, http.StatusServiceUnavailable, "points are not configured")
		return
	}
	claims, _ := authjwt.FromContext(r.Context())
	switch {
	case len(rest) >= 1 && rest[0] == "me":
		if claims.UserID == "" || claims.AccountType == serviceaccount.AccountType {
			respondError(w, http.StatusForbidden, "forbidden: customer account required")
			return
		}
		h.pointsAccount(w, r, claims.UserID, rest[1:])
	case len(rest) >= 2 && rest[0] == "customers":
		if len(rest) == 3 && rest[2] == "adjust" {
			if r.Method != http.MethodPost {
				respondError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if !permissions.CanManagePoints(claims.Permissions) {
				respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
				return
			}
			h.adjustPoints(w, r, rest[1], operatorName(claims))
			return
		}
		if !permissions.CanReadPoints(claims.Permissions) {
			respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
			return
		}
		h.pointsAccount(w, r, rest[1], rest[2:])
	case len(rest) == 1 && (rest[0] == "reserve" || rest[0] == "consume" || rest[0] == "release"):
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !permissions.CanRedeemPoints(claims.Permissions) || !claims.CalledBy(serviceaccount.Order) {
			respondError(w, http.StatusForbidden, "forbidden: internal API")
			return
		}
		h.pointsHold(w, r, rest[0])
	default:
		respondError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) pointsAccount(w http.ResponseWriter, r *http.Request, customerID string, rest []string) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	switch {
	case len(rest) == 0:
		summary, err := h.points.Summary(r.Context(), customerID, limit)
		if err != nil {
			respondPointsError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, summary)
	case len(rest) == 1 && rest[0] == "entries":
		entries, next, err := h.points.Entries(r.Context(), customerID, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			respondPointsError(w, err)
			return
		}
		body := map[string]any{"entries": entries}
		if next != "" {
			body["next_cursor"] = next
		}
		respondJSON(w, http.StatusOK, body)
	default:
		respondError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) adjustPoints(w http.ResponseWriter, r *http.Request, customerID, by string) {
	var req struct {
		Points int64  `json:"points"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := h.points.Adjust(r.Context(), service.AdjustInput{
		CustomerID: customerID,
		Points:     req.Points,
		Reason:     req.Reason,
		Ref:        r.Header.Get("Idempotency-Key"),
		By:         by,
	})
	if err != nil {
		respondPointsError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, summary)
}

func (h *Handler) pointsHold(w http.ResponseWriter, r *http.Request, action string) {
	var req struct {
		OrderID    string `json:"order_id"`
		CustomerID string `json:"customer_id"`
		Points     int64  `json:"points"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	var (
		hold *domain.PointHold
		err  error
	)
	switch action {
	case "reserve":
		hold, err = h.points.Reserve(r.Context(), req.OrderID, req.CustomerID, req.Points)
	case "consume":
		hold, err = h.points.Consume(r.Context(), req.OrderID)
	case "release":
		hold, err = h.points.Release(r.Context(), req.OrderID)
	}
	if err != nil {
		respondPointsError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, hold)
}

// respondPointsError maps ledger errors; reason lets order tell them apart
// without parsing messages.
func respondPointsError(w http.ResponseWriter, err error) {
	reason := func(status int, reason string) {
		respondJSON(w, status, map[string]any{"error": err.Error(), "code": status, "reason": reason})
	}
	switch {
	case errors.Is(err, domain.ErrInvalidPoints):
		reason(http.StatusBadRequest, "invalid_points")
	case errors.Is(err, domain.ErrInsufficientPoints):
		reason(http.StatusConflict, "insufficient_points")
	case errors.Is(err, domain.ErrPointsHoldConflict):
		reason(http.StatusConflict, "points_hold_conflict")
	case errors.Is(err, domain.ErrPointsHoldClosed):
		reason(http.StatusConflict, "points_hold_closed")
	case errors.Is(err, domain.ErrPointsHoldNotFound):
		reason(http.StatusNotFound, "points_hold_not_found")
	default:
		respondServiceError(w, err)
	}
}

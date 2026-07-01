package handler

import (
	"net/http"
	"strings"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/service"
)

func (h *Handler) checkoutSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		CustomerID string `json:"customer_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	session, err := h.svc.CreateCheckoutSession(r.Context(), service.CreateCheckoutSessionInput{
		CustomerID: req.CustomerID,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, session)
}

func (h *Handler) checkoutSession(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/v1/checkout/sessions/"))
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	sessionID := parts[0]

	if len(parts) == 1 && r.Method == http.MethodGet {
		session, err := h.svc.GetCheckoutSession(r.Context(), sessionID)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, session)
		return
	}

	if len(parts) == 2 && parts[1] == "complete" && r.Method == http.MethodPost {
		result, err := h.svc.CompleteCheckout(r.Context(), sessionID)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, result)
		return
	}

	if len(parts) == 2 && parts[1] == "coupon" && r.Method == http.MethodPost {
		var req struct {
			Code string `json:"code"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		session, err := h.svc.ApplyCheckoutCoupon(r.Context(), sessionID, req.Code)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, session)
		return
	}

	if len(parts) == 2 && parts[1] == "items" {
		switch r.Method {
		case http.MethodPut:
			var req struct {
				Items []domain.OrderItem `json:"items"`
			}
			if err := decodeJSON(r, &req); err != nil {
				respondError(w, http.StatusBadRequest, err.Error())
				return
			}
			session, err := h.svc.SetCheckoutItems(r.Context(), sessionID, req.Items)
			if err != nil {
				respondServiceError(w, err)
				return
			}
			respondJSON(w, http.StatusOK, session)
		case http.MethodPost:
			var item domain.OrderItem
			if err := decodeJSON(r, &item); err != nil {
				respondError(w, http.StatusBadRequest, err.Error())
				return
			}
			session, err := h.svc.UpsertCheckoutItem(r.Context(), sessionID, item)
			if err != nil {
				respondServiceError(w, err)
				return
			}
			respondJSON(w, http.StatusOK, session)
		default:
			respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 3 && parts[1] == "items" && r.Method == http.MethodDelete {
		session, err := h.svc.RemoveCheckoutItem(r.Context(), sessionID, parts[2])
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, session)
		return
	}

	respondError(w, http.StatusNotFound, "not found")
}

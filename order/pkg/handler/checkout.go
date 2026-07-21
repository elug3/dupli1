package handler

import (
	"net/http"
	"strings"

	"github.com/elug3/dupli1/order/pkg/authjwt"
	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func (h *Handler) checkoutSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, _ := authjwt.FromContext(r.Context())

	var req struct {
		CustomerID string `json:"customer_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.jwtValidator != nil && !permissions.BypassesOrderCreateABAC(claims.Permissions) {
		if req.CustomerID != claims.UserID {
			respondError(w, http.StatusForbidden, "forbidden: customer_id must match your user id")
			return
		}
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
	claims, _ := authjwt.FromContext(r.Context())

	parts := checkoutSessionPathParts(r.URL.Path)
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
		if !h.mayAccessCheckoutSession(claims, session.CustomerID, false) {
			respondError(w, http.StatusForbidden, "forbidden: you do not own this checkout session")
			return
		}
		respondJSON(w, http.StatusOK, session)
		return
	}

	if len(parts) == 2 && parts[1] == "complete" && r.Method == http.MethodPost {
		if err := h.withCheckoutSessionAccess(w, r, claims, sessionID, true); err != nil {
			return
		}
		result, err := h.svc.CompleteCheckout(r.Context(), sessionID)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, result)
		return
	}

	if len(parts) == 2 && parts[1] == "coupon" && r.Method == http.MethodPost {
		if err := h.withCheckoutSessionAccess(w, r, claims, sessionID, true); err != nil {
			return
		}
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
		if err := h.withCheckoutSessionAccess(w, r, claims, sessionID, true); err != nil {
			return
		}
		switch r.Method {
		case http.MethodPut:
			var req struct {
				Items []struct {
					SkuID    string `json:"sku_id"`
					SKU      string `json:"sku"`
					Quantity int    `json:"quantity"`
				} `json:"items"`
			}
			if err := decodeJSON(r, &req); err != nil {
				respondError(w, http.StatusBadRequest, err.Error())
				return
			}
			items := make([]domain.OrderItem, len(req.Items))
			for i, item := range req.Items {
				items[i] = domain.OrderItem{SkuID: item.SkuID, SKU: item.SKU, Quantity: item.Quantity}
			}
			session, err := h.svc.SetCheckoutItems(r.Context(), sessionID, items)
			if err != nil {
				respondServiceError(w, err)
				return
			}
			respondJSON(w, http.StatusOK, session)
		case http.MethodPost:
			var req struct {
				SkuID    string `json:"sku_id"`
				SKU      string `json:"sku"`
				Quantity int    `json:"quantity"`
			}
			if err := decodeJSON(r, &req); err != nil {
				respondError(w, http.StatusBadRequest, err.Error())
				return
			}
			session, err := h.svc.UpsertCheckoutItem(r.Context(), sessionID, domain.OrderItem{
				SkuID:    req.SkuID,
				SKU:      req.SKU,
				Quantity: req.Quantity,
			})
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

	if len(parts) == 4 && parts[1] == "items" && parts[2] == "by-sku-id" && r.Method == http.MethodDelete {
		if err := h.withCheckoutSessionAccess(w, r, claims, sessionID, true); err != nil {
			return
		}
		session, err := h.svc.RemoveCheckoutItemBySkuID(r.Context(), sessionID, parts[3])
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, session)
		return
	}

	if len(parts) == 3 && parts[1] == "items" && r.Method == http.MethodDelete {
		if err := h.withCheckoutSessionAccess(w, r, claims, sessionID, true); err != nil {
			return
		}
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

func (h *Handler) mayAccessCheckoutSession(claims authjwt.Claims, sessionCustomerID string, requireCreateBypass bool) bool {
	if h.jwtValidator == nil {
		return false
	}
	if requireCreateBypass && permissions.BypassesOrderCreateABAC(claims.Permissions) {
		return true
	}
	if !requireCreateBypass && permissions.BypassesOrderReadABAC(claims.Permissions) {
		return true
	}
	return sessionCustomerID == claims.UserID
}

func (h *Handler) withCheckoutSessionAccess(w http.ResponseWriter, r *http.Request, claims authjwt.Claims, sessionID string, requireCreateBypass bool) error {
	session, err := h.svc.GetCheckoutSession(r.Context(), sessionID)
	if err != nil {
		respondServiceError(w, err)
		return err
	}
	if !h.mayAccessCheckoutSession(claims, session.CustomerID, requireCreateBypass) {
		respondError(w, http.StatusForbidden, "forbidden: you do not own this checkout session")
		return errForbidden
	}
	return nil
}

var errForbidden = &forbiddenError{}

type forbiddenError struct{}

func (e *forbiddenError) Error() string { return "forbidden" }

func checkoutSessionPathParts(path string) []string {
	for _, prefix := range []string{
		"/api/v1/orders/checkout/sessions/",
		"/api/v1/checkout/sessions/",
	} {
		if strings.HasPrefix(path, prefix) {
			return splitPath(strings.TrimPrefix(path, prefix))
		}
	}
	return nil
}

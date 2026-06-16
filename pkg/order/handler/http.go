package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/elug3/schick/pkg/order/domain"
	"github.com/elug3/schick/pkg/order/ports"
	"github.com/elug3/schick/pkg/order/service"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/orders", h.orders)
	mux.HandleFunc("/api/v1/orders/", h.order)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) orders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			CustomerID string             `json:"customer_id"`
			Items      []domain.OrderItem `json:"items"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		order, err := h.svc.CreateOrder(r.Context(), service.CreateOrderInput{
			CustomerID: req.CustomerID,
			Items:      req.Items,
		})
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusCreated, order)
	case http.MethodGet:
		customerID := r.URL.Query().Get("customer_id")
		if customerID == "" {
			respondError(w, http.StatusBadRequest, "customer_id query parameter is required")
			return
		}
		orders, err := h.svc.ListCustomerOrders(r.Context(), customerID)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"total":  len(orders),
			"orders": orders,
		})
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) order(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/v1/orders/"))
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		order, err := h.svc.GetOrder(r.Context(), parts[0])
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, order)
		return
	}

	if len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodPut {
		var req struct {
			Status domain.OrderStatus `json:"status"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		var (
			order *domain.Order
			err   error
		)
		switch req.Status {
		case domain.StatusConfirmed:
			order, err = h.svc.ConfirmOrder(r.Context(), parts[0])
		case domain.StatusCanceled:
			order, err = h.svc.CancelOrder(r.Context(), parts[0])
		case domain.StatusFulfilled:
			order, err = h.svc.FulfillOrder(r.Context(), parts[0])
		default:
			respondError(w, http.StatusBadRequest, "unsupported status")
			return
		}
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, order)
		return
	}

	respondError(w, http.StatusNotFound, "not found")
}

func respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidOrder), errors.Is(err, domain.ErrInvalidTransition):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, err.Error())
	}
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]any{
		"error": message,
		"code":  status,
	})
}

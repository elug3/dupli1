package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/elug3/schick/pkg/inventory/domain"
	"github.com/elug3/schick/pkg/inventory/ports"
	"github.com/elug3/schick/pkg/inventory/service"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/api/v1/inventory/reservations", h.reservations)
	mux.HandleFunc("/api/v1/inventory/reservations/", h.reservation)
	mux.HandleFunc("/api/v1/inventory/", h.item)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) item(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/v1/inventory/"))
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	sku := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			item, err := h.svc.GetItem(r.Context(), sku)
			if err != nil {
				respondServiceError(w, err)
				return
			}
			respondJSON(w, http.StatusOK, item)
		case http.MethodPut:
			var req struct {
				Quantity int `json:"quantity"`
			}
			if err := decodeJSON(r, &req); err != nil {
				respondError(w, http.StatusBadRequest, err.Error())
				return
			}
			item, err := h.svc.UpsertItem(r.Context(), sku, req.Quantity)
			if err != nil {
				respondServiceError(w, err)
				return
			}
			respondJSON(w, http.StatusOK, item)
		default:
			respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "adjust" && r.Method == http.MethodPost {
		var req struct {
			Delta int `json:"delta"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		item, err := h.svc.AdjustStock(r.Context(), sku, req.Delta)
		if err != nil {
			respondServiceError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, item)
		return
	}

	respondError(w, http.StatusNotFound, "not found")
}

func (h *Handler) reservations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		OrderID string                   `json:"order_id"`
		Items   []domain.ReservationItem `json:"items"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	reservation, err := h.svc.Reserve(r.Context(), req.OrderID, req.Items)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{
		"reservation_id": reservation.ID,
		"reservation":    reservation,
	})
}

func (h *Handler) reservation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/api/v1/inventory/reservations/"))
	if len(parts) != 2 {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	var (
		reservation *domain.Reservation
		err         error
	)
	switch parts[1] {
	case "commit":
		reservation, err = h.svc.CommitReservation(r.Context(), parts[0])
	case "release":
		reservation, err = h.svc.ReleaseReservation(r.Context(), parts[0])
	default:
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, reservation)
}

func respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrInvalidSKU), errors.Is(err, service.ErrInvalidQuantity), errors.Is(err, service.ErrInsufficientStock), errors.Is(err, service.ErrReservationClosed):
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

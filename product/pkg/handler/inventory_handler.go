package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
)

// UpsertInventoryItemHandler returns an http.Handler for PUT /api/v1/inventory/{sku}.
func (h *Handler) UpsertInventoryItemHandler() http.Handler {
	return http.HandlerFunc(h.UpsertInventoryItem)
}

// AdjustInventoryItemHandler returns an http.Handler for POST /api/v1/inventory/{sku}/adjust.
func (h *Handler) AdjustInventoryItemHandler() http.Handler {
	return http.HandlerFunc(h.AdjustInventoryItem)
}

// UpsertInventoryItemBySkuIDHandler returns an http.Handler for PUT /api/v1/inventory/by-sku-id/{skuId}.
func (h *Handler) UpsertInventoryItemBySkuIDHandler() http.Handler {
	return http.HandlerFunc(h.UpsertInventoryItemBySkuID)
}

// AdjustInventoryItemBySkuIDHandler returns an http.Handler for POST /api/v1/inventory/by-sku-id/{skuId}/adjust.
func (h *Handler) AdjustInventoryItemBySkuIDHandler() http.Handler {
	return http.HandlerFunc(h.AdjustInventoryItemBySkuID)
}

// CreateReservationHandler returns an http.Handler for POST /api/v1/inventory/reservations.
func (h *Handler) CreateReservationHandler() http.Handler {
	return http.HandlerFunc(h.CreateReservation)
}

// CommitReservationHandler returns an http.Handler for POST /api/v1/inventory/reservations/{id}/commit.
func (h *Handler) CommitReservationHandler() http.Handler {
	return http.HandlerFunc(h.CommitReservation)
}

// ReleaseReservationHandler returns an http.Handler for POST /api/v1/inventory/reservations/{id}/release.
func (h *Handler) ReleaseReservationHandler() http.Handler {
	return http.HandlerFunc(h.ReleaseReservation)
}

func (h *Handler) GetInventoryItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sku := r.PathValue("sku")
	item, err := h.inventorySvc.GetItem(r.Context(), service.SkuRef{SKU: sku})
	if err != nil {
		h.respondInventoryError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, item)
}

func (h *Handler) GetInventoryItemBySkuID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	skuID := r.PathValue("skuId")
	item, err := h.inventorySvc.GetItem(r.Context(), service.SkuRef{SkuID: skuID})
	if err != nil {
		h.respondInventoryError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, item)
}

func (h *Handler) UpsertInventoryItem(w http.ResponseWriter, r *http.Request) {
	h.upsertInventoryItem(w, r, service.SkuRef{SKU: r.PathValue("sku")})
}

func (h *Handler) UpsertInventoryItemBySkuID(w http.ResponseWriter, r *http.Request) {
	h.upsertInventoryItem(w, r, service.SkuRef{SkuID: r.PathValue("skuId")})
}

func (h *Handler) upsertInventoryItem(w http.ResponseWriter, r *http.Request, ref service.SkuRef) {
	var req struct {
		Quantity int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.inventorySvc.UpsertItem(r.Context(), ref, req.Quantity)
	if err != nil {
		h.respondInventoryError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, item)
}

func (h *Handler) AdjustInventoryItem(w http.ResponseWriter, r *http.Request) {
	h.adjustInventoryItem(w, r, service.SkuRef{SKU: r.PathValue("sku")})
}

func (h *Handler) AdjustInventoryItemBySkuID(w http.ResponseWriter, r *http.Request) {
	h.adjustInventoryItem(w, r, service.SkuRef{SkuID: r.PathValue("skuId")})
}

func (h *Handler) adjustInventoryItem(w http.ResponseWriter, r *http.Request, ref service.SkuRef) {
	var req struct {
		Delta int `json:"delta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.inventorySvc.AdjustStock(r.Context(), ref, req.Delta)
	if err != nil {
		h.respondInventoryError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateReservation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string `json:"order_id"`
		Items   []struct {
			SKU      string `json:"sku"`
			SkuID    string `json:"skuId"`
			Quantity int    `json:"quantity"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	items := make([]service.ReservationItemRef, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.ReservationItemRef{
			Ref:      service.SkuRef{SkuID: it.SkuID, SKU: it.SKU},
			Quantity: it.Quantity,
		}
	}
	reservation, err := h.inventorySvc.Reserve(r.Context(), req.OrderID, items)
	if err != nil {
		h.respondInventoryError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, map[string]any{
		"reservation_id": reservation.ID,
		"reservation":    reservation,
	})
}

func (h *Handler) CommitReservation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reservation, err := h.inventorySvc.CommitReservation(r.Context(), id)
	if err != nil {
		h.respondInventoryError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, reservation)
}

func (h *Handler) ReleaseReservation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reservation, err := h.inventorySvc.ReleaseReservation(r.Context(), id)
	if err != nil {
		h.respondInventoryError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, reservation)
}

func (h *Handler) respondInventoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrInventoryItemNotFound):
		h.respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrInvalidSKU),
		errors.Is(err, service.ErrInvalidQuantity),
		errors.Is(err, service.ErrInsufficientStock),
		errors.Is(err, service.ErrReservationClosed):
		h.respondError(w, http.StatusBadRequest, err.Error())
	default:
		h.respondError(w, http.StatusInternalServerError, err.Error())
	}
}

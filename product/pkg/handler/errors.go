package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/money"
)

// respondBodyError answers a product or variant body that failed to decode.
// A price money.Won refused (fractional or out of range) gets the same 400 a
// negative price gets from the service, naming the field; anything else is
// the generic invalid-body answer.
func (h *Handler) respondBodyError(w http.ResponseWriter, err error) {
	if ute, ok := money.IsWonNumberError(err); ok {
		h.respondServiceError(w, ports.Invalid(domain.WholeWonError(ute.Field).Error()))
		return
	}
	h.respondError(w, http.StatusBadRequest, "invalid request body")
}

// respondServiceError maps classified service/store errors to HTTP responses.
// Known client errors expose their message; unexpected failures (including raw
// database errors) return a generic 500 so SQL details never leak to clients.
func (h *Handler) respondServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound),
		errors.Is(err, ports.ErrInventoryItemNotFound),
		errors.Is(err, domain.ErrMasterNotFound):
		h.respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ports.ErrConflict),
		errors.Is(err, domain.ErrMasterExists),
		errors.Is(err, domain.ErrMasterInUse):
		h.respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ports.ErrInvalid),
		errors.Is(err, domain.ErrMissingSKUCodes),
		errors.Is(err, service.ErrInvalidSKU),
		errors.Is(err, service.ErrInvalidQuantity),
		errors.Is(err, service.ErrInsufficientStock),
		errors.Is(err, service.ErrReservationClosed),
		errors.Is(err, service.ErrReservationAlreadyCommitted),
		errors.Is(err, service.ErrReservationAlreadyReleased),
		errors.Is(err, ports.ErrInsufficientStock),
		errors.Is(err, ports.ErrReservationClosed),
		errors.Is(err, ports.ErrReservationAlreadyCommitted),
		errors.Is(err, ports.ErrReservationAlreadyReleased):
		h.respondError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("product: internal error: %v", err)
		h.respondError(w, http.StatusInternalServerError, "internal error")
	}
}

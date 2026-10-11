package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/service"
)

// WithCatalogReportService enables the catalog overview report.
func (h *Handler) WithCatalogReportService(svc *service.CatalogReportService) *Handler {
	h.catalogReportSvc = svc
	return h
}

// CatalogReport serves GET /api/v1/products/reports/catalog?status=active|
// draft|archived (optional): products, SKUs and stock per brand, category,
// subcategory and brand × subcategory. Requires product.read.
func (h *Handler) CatalogReport(w http.ResponseWriter, r *http.Request) {
	if h.catalogReportSvc == nil {
		h.respondError(w, http.StatusServiceUnavailable, "catalog report unavailable")
		return
	}
	report, err := h.catalogReportSvc.Report(r.Context(), r.URL.Query().Get("status"))
	switch {
	case errors.Is(err, service.ErrInvalidProductStatus):
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		log.Printf("catalog report: %v", err)
		h.respondError(w, http.StatusInternalServerError, "catalog report failed")
		return
	}
	h.respondJSON(w, http.StatusOK, report)
}

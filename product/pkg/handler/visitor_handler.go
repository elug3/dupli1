package handler

import (
	"errors"
	"log"
	"net/http"
	"regexp"

	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// botUserAgent matches crawlers, link unfurlers and headless browsers. The
// storefront records a visit from a script after the page loads, which most
// crawlers never run; this catches the ones that do.
var botUserAgent = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|preview|headless|lighthouse|facebookexternalhit|embedly|python-requests|curl|wget`)

// WithVisitorService enables POST /visits and the visitors report.
func (h *Handler) WithVisitorService(svc *service.VisitorService) *Handler {
	h.visitorSvc = svc
	return h
}

// RecordVisit serves POST /api/v1/products/visits, the storefront's
// once-per-page-load beacon. It reads the dupli1_guest cookie, minting it
// when absent (the same cookie unique product views use), and counts the
// browser once for today (KST). Always 204: a visit nobody counted is not
// the shopper's problem, so a bot, a prefetch or a store error is silent.
func (h *Handler) RecordVisit(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if h.visitorSvc == nil || !h.guestCookie.Enabled || !countableVisit(r) {
		return
	}
	guestID, minted := h.ensureGuestID(r)
	if minted {
		h.setGuestCookie(w, guestID)
	}
	if _, err := h.visitorSvc.RecordVisit(r.Context(), guestID); err != nil {
		log.Printf("record visit: %v", err)
	}
}

func countableVisit(r *http.Request) bool {
	if r.Header.Get("Sec-Purpose") != "" || r.Header.Get("Purpose") == "prefetch" {
		return false
	}
	ua := r.Header.Get("User-Agent")
	return ua != "" && !botUserAgent.MatchString(ua)
}

// VisitorReport serves GET /api/v1/products/reports/visitors?granularity=
// week|month&from=YYYY-MM-DD&to=YYYY-MM-DD (KST, both optional): unique
// storefront visitors per period. Requires product.read.
func (h *Handler) VisitorReport(w http.ResponseWriter, r *http.Request) {
	if h.visitorSvc == nil {
		h.respondError(w, http.StatusServiceUnavailable, "visitor reports unavailable")
		return
	}
	q := r.URL.Query()
	report, err := h.visitorSvc.Report(r.Context(), q.Get("granularity"), q.Get("from"), q.Get("to"))
	switch {
	case errors.Is(err, reportperiod.ErrInvalidRange):
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		log.Printf("visitor report: %v", err)
		h.respondError(w, http.StatusInternalServerError, "visitor report failed")
		return
	}
	h.respondJSON(w, http.StatusOK, report)
}

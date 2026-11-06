package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// The server's ReadTimeout (5s) and WriteTimeout (25s) suit JSON calls, not a
// phone video crossing a slow uplink or a shopper scrubbing through one.
// These two routes lift them for their own request only.
const (
	inspectionVideoUploadTimeout = 15 * time.Minute
	inspectionVideoStreamTimeout = 30 * time.Minute
)

// inspectionVideo serves /api/v1/orders/{id}/inspection-video.
func (h *Handler) inspectionVideo(w http.ResponseWriter, r *http.Request, orderID string) {
	switch r.Method {
	case http.MethodPut:
		h.uploadInspectionVideo(w, r, orderID)
	case http.MethodGet, http.MethodHead:
		h.streamInspectionVideo(w, r, orderID)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// uploadInspectionVideo takes the video as the raw request body (not
// multipart), so it streams straight into object storage. Content-Type must
// be video/mp4, video/quicktime or video/webm and Content-Length is required.
// Gated on order.ship: the staff who ship are the ones who film.
func (h *Handler) uploadInspectionVideo(w http.ResponseWriter, r *http.Request, orderID string) {
	claims, _ := authjwt.FromContext(r.Context())
	if h.jwtValidator != nil && !claims.HasPermission(permissions.OrderShip) {
		respondError(w, http.StatusForbidden, "forbidden: insufficient permission")
		return
	}
	defer r.Body.Close()

	if r.ContentLength < 0 {
		respondError(w, http.StatusLengthRequired, "Content-Length is required")
		return
	}
	maxBytes := h.svc.InspectionVideoMaxBytes()
	if r.ContentLength > maxBytes {
		respondError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the video is larger than %d bytes", maxBytes))
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(inspectionVideoUploadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(inspectionVideoUploadTimeout))

	order, err := h.svc.UploadInspectionVideo(r.Context(), service.UploadInspectionVideoInput{
		OrderID:     orderID,
		UploadedBy:  claims.UserID,
		ContentType: r.Header.Get("Content-Type"),
		SizeBytes:   r.ContentLength,
		Body:        http.MaxBytesReader(w, r.Body, r.ContentLength),
	})
	if err != nil {
		respondInspectionVideoError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, order)
}

// streamInspectionVideo plays the video back. Staff with order.read.all see it
// once attached; the order's customer from shipping on. Range requests are
// honoured, so a player can seek.
func (h *Handler) streamInspectionVideo(w http.ResponseWriter, r *http.Request, orderID string) {
	claims, _ := authjwt.FromContext(r.Context())
	access := service.InspectionVideoAccess{
		UserID: claims.UserID,
		Staff:  h.jwtValidator == nil || permissions.BypassesOrderReadABAC(claims.Permissions),
	}
	video, obj, err := h.svc.OpenInspectionVideo(r.Context(), orderID, access)
	if err != nil {
		respondInspectionVideoError(w, err)
		return
	}
	defer obj.Close()

	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(inspectionVideoStreamTimeout))
	w.Header().Set("Content-Type", video.ContentType)
	// The customer's own evidence: never kept by a shared cache.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", obj.ModTime(), obj)
}

func respondInspectionVideoError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		respondError(w, http.StatusRequestEntityTooLarge, "the video is larger than its Content-Length")
	case errors.Is(err, service.ErrInspectionVideoForbidden):
		respondError(w, http.StatusForbidden, err.Error())
	default:
		respondServiceError(w, err)
	}
}

// inspectionVideoErrorStatus maps the inspection video errors that can also
// come back from the ship call; used by respondServiceError.
func inspectionVideoErrorStatus(err error) (int, bool) {
	switch {
	case errors.Is(err, domain.ErrInspectionVideoRequired), errors.Is(err, domain.ErrInspectionVideoLocked):
		return http.StatusConflict, true
	case errors.Is(err, domain.ErrInvalidInspectionVideo):
		return http.StatusBadRequest, true
	case errors.Is(err, ports.ErrInspectionVideoStorageUnavailable):
		return http.StatusServiceUnavailable, true
	}
	return 0, false
}

// inspectionVideoReason is the machine-readable reason a client branches on
// (manage-web shows "attach the 검수영상 first" for inspection_video_required).
func inspectionVideoReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrInspectionVideoRequired):
		return "inspection_video_required"
	case errors.Is(err, domain.ErrInspectionVideoLocked):
		return "inspection_video_locked"
	case errors.Is(err, domain.ErrInvalidInspectionVideo):
		return "invalid_inspection_video"
	default:
		return "inspection_video_storage_unavailable"
	}
}

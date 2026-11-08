package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

// Review routes (docs/product-reviews-plan.md). Review ids sit behind a
// literal by-id segment so they cannot overlap /products/{id}/... patterns.
const (
	RouteProductReviews      = "/api/v1/products/{id}/reviews"
	RouteProductReviewPhotos = "/api/v1/products/{id}/reviews/photos"
	RouteReviewPhotoUpload   = "/api/v1/products/reviews/photos"
	RouteMyReviews           = "/api/v1/products/reviews/me"
	RouteReviews             = "/api/v1/products/reviews"
	RouteReviewByID          = "/api/v1/products/reviews/by-id/{reviewId}"
	RouteReviewHide          = "/api/v1/products/reviews/by-id/{reviewId}/hide"
)

// reviewPhotoBodyLimit caps the multipart body: one photo plus the form.
const reviewPhotoBodyLimit = service.ReviewPhotoMaxBytes + 1<<20

// WithReviewService enables the review routes.
func (h *Handler) WithReviewService(svc *service.ReviewService) *Handler {
	h.reviewSvc = svc
	return h
}

// reviewErrorResponse is product's error shape plus a machine-readable reason.
type reviewErrorResponse struct {
	Error    string `json:"error"`
	Code     int    `json:"code"`
	Reason   string `json:"reason"`
	ReviewID string `json:"reviewId,omitempty"`
}

func (h *Handler) respondReviewError(w http.ResponseWriter, err error) {
	var exists *ports.ReviewExistsError
	resp := func(status int, reason, msg string) {
		h.respondJSON(w, status, reviewErrorResponse{Error: msg, Code: status, Reason: reason})
	}
	switch {
	case errors.As(err, &exists):
		h.respondJSON(w, http.StatusConflict, reviewErrorResponse{
			Error: "this order line already has a review", Code: http.StatusConflict,
			Reason: "review_exists", ReviewID: exists.ReviewID,
		})
	case errors.Is(err, domain.ErrInvalidReview):
		resp(http.StatusBadRequest, "invalid_review", err.Error())
	case errors.Is(err, ports.ErrReviewPhotoUnusable):
		resp(http.StatusBadRequest, "invalid_review", "a photo id is not yours or is already used")
	case errors.Is(err, service.ErrInvalidReviewPhoto):
		resp(http.StatusBadRequest, "invalid_photo", err.Error())
	case errors.Is(err, ports.ErrInvalid):
		resp(http.StatusBadRequest, "invalid_review", err.Error())
	case errors.Is(err, service.ErrCustomerRequired):
		resp(http.StatusForbidden, "customer_required", "a customer account is required")
	case errors.Is(err, service.ErrNotReviewable):
		resp(http.StatusForbidden, "not_reviewable", "this order line cannot be reviewed")
	case errors.Is(err, ports.ErrReviewNotFound):
		resp(http.StatusNotFound, "review_not_found", "review not found")
	case errors.Is(err, ports.ErrNotFound):
		resp(http.StatusNotFound, "product_not_found", "product not found")
	case errors.Is(err, service.ErrReviewHidden):
		resp(http.StatusConflict, "review_hidden", "a hidden review cannot be edited")
	case errors.Is(err, ports.ErrReferenceUnavailable):
		log.Printf("product: review reference unavailable: %v", err)
		resp(http.StatusServiceUnavailable, "reference_unavailable", "order lookup is unavailable; try again")
	case errors.Is(err, service.ErrReviewsUnavailable):
		resp(http.StatusServiceUnavailable, "reviews_unavailable", "reviews are unavailable")
	default:
		log.Printf("product: review internal error: %v", err)
		resp(http.StatusServiceUnavailable, "reviews_unavailable", "reviews are unavailable")
	}
}

func (h *Handler) reviewsReady(w http.ResponseWriter) bool {
	if h.reviewSvc == nil {
		h.respondReviewError(w, service.ErrReviewsUnavailable)
		return false
	}
	return true
}

// reviewCaller reads the signed-in shopper and their own token.
func reviewCaller(r *http.Request) service.ReviewCaller {
	claims, _ := authjwt.FromContext(r.Context())
	bearer := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(bearer) > 7 && strings.EqualFold(bearer[:7], "bearer ") {
		bearer = strings.TrimSpace(bearer[7:])
	} else {
		bearer = ""
	}
	return service.ReviewCaller{CustomerID: claims.UserID, AccountType: claims.AccountType, Bearer: bearer}
}

func queryInt(r *http.Request, key string) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil
}

func decodeReviewBody(r *http.Request, out any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return ports.Invalid("invalid JSON body")
	}
	return nil
}

// ListProductReviews serves GET /api/v1/products/{id}/reviews (public).
func (h *Handler) ListProductReviews(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	q := r.URL.Query()
	rating, ok := queryInt(r, "rating")
	limit, ok2 := queryInt(r, "limit")
	if !ok || !ok2 {
		h.respondReviewError(w, ports.Invalid("rating and limit must be numbers"))
		return
	}
	photo := strings.EqualFold(q.Get("photo"), "true") || q.Get("photo") == "1"
	out, err := h.reviewSvc.ProductReviews(r.Context(), r.PathValue("id"), domain.ReviewQuery{
		Sort: q.Get("sort"), PhotoOnly: photo, Rating: rating, Limit: limit, Cursor: q.Get("cursor"),
	})
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	h.respondJSON(w, http.StatusOK, out)
}

// ListProductReviewPhotos serves GET /api/v1/products/{id}/reviews/photos.
func (h *Handler) ListProductReviewPhotos(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	limit, ok := queryInt(r, "limit")
	if !ok {
		h.respondReviewError(w, ports.Invalid("limit must be a number"))
		return
	}
	page, err := h.reviewSvc.ProductReviewPhotos(r.Context(), r.PathValue("id"), limit, r.URL.Query().Get("cursor"))
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	type photo struct {
		ReviewID string `json:"reviewId"`
		ID       string `json:"id"`
		URL      string `json:"url"`
		ThumbURL string `json:"thumbUrl"`
	}
	photos := make([]photo, 0, len(page.Photos))
	for _, p := range page.Photos {
		photos = append(photos, photo{ReviewID: p.ReviewID, ID: p.ID, URL: p.URL, ThumbURL: p.ThumbURL})
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	h.respondJSON(w, http.StatusOK, map[string]any{"photos": photos, "nextCursor": page.NextCursor})
}

// UploadReviewPhoto serves POST /api/v1/products/reviews/photos (multipart
// field "photo").
func (h *Handler) UploadReviewPhoto(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	if !h.reviewSvc.PhotosEnabled() {
		h.respondReviewError(w, service.ErrReviewsUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, reviewPhotoBodyLimit)
	if err := r.ParseMultipartForm(reviewPhotoBodyLimit); err != nil {
		h.respondReviewError(w, fmt.Errorf("%w: send one photo of at most 10 MB as multipart field \"photo\"", service.ErrInvalidReviewPhoto))
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		h.respondReviewError(w, fmt.Errorf("%w: missing photo field", service.ErrInvalidReviewPhoto))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, service.ReviewPhotoMaxBytes+1))
	if err != nil {
		h.respondReviewError(w, fmt.Errorf("%w: %v", service.ErrInvalidReviewPhoto, err))
		return
	}
	photo, err := h.reviewSvc.UploadPhoto(r.Context(), reviewCaller(r), data, header.Header.Get("Content-Type"))
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, map[string]string{
		"photoId": photo.ID, "url": photo.URL, "thumbUrl": photo.ThumbURL,
	})
}

// CreateReview serves POST /api/v1/products/{id}/reviews.
func (h *Handler) CreateReview(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	var in service.CreateReviewInput
	if err := decodeReviewBody(r, &in); err != nil {
		h.respondReviewError(w, err)
		return
	}
	review, err := h.reviewSvc.Create(r.Context(), reviewCaller(r), r.PathValue("id"), in)
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, review)
}

// MyReviews serves GET /api/v1/products/reviews/me[?orderId=].
func (h *Handler) MyReviews(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	reviews, err := h.reviewSvc.Mine(r.Context(), reviewCaller(r), r.URL.Query().Get("orderId"))
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, map[string]any{"reviews": reviews})
}

// UpdateReview serves PATCH /api/v1/products/reviews/by-id/{reviewId}.
func (h *Handler) UpdateReview(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	var in domain.ReviewInput
	if err := decodeReviewBody(r, &in); err != nil {
		h.respondReviewError(w, err)
		return
	}
	review, err := h.reviewSvc.Update(r.Context(), reviewCaller(r), r.PathValue("reviewId"), in)
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, review)
}

// DeleteReview serves DELETE /api/v1/products/reviews/by-id/{reviewId}.
func (h *Handler) DeleteReview(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	if err := h.reviewSvc.Delete(r.Context(), reviewCaller(r), r.PathValue("reviewId")); err != nil {
		h.respondReviewError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ModerateReviews serves GET /api/v1/products/reviews (staff).
func (h *Handler) ModerateReviews(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	q := r.URL.Query()
	rating, ok := queryInt(r, "rating")
	limit, ok2 := queryInt(r, "limit")
	if !ok || !ok2 {
		h.respondReviewError(w, ports.Invalid("rating and limit must be numbers"))
		return
	}
	var hidden bool
	switch q.Get("status") {
	case "", "visible":
	case "hidden":
		hidden = true
	default:
		h.respondReviewError(w, ports.Invalid("status must be visible or hidden"))
		return
	}
	page, err := h.reviewSvc.Moderation(r.Context(), domain.ModerationQuery{
		Hidden: hidden, ProductID: strings.TrimSpace(q.Get("productId")), Rating: rating, Limit: limit, Cursor: q.Get("cursor"),
	})
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, map[string]any{"reviews": page.Reviews, "nextCursor": page.NextCursor})
}

// HideReview serves POST /api/v1/products/reviews/by-id/{reviewId}/hide.
func (h *Handler) HideReview(w http.ResponseWriter, r *http.Request) {
	if !h.reviewsReady(w) {
		return
	}
	body := struct {
		Hidden *bool  `json:"hidden"`
		Reason string `json:"reason"`
	}{}
	if err := decodeReviewBody(r, &body); err != nil {
		h.respondReviewError(w, err)
		return
	}
	hidden := body.Hidden == nil || *body.Hidden
	claims, _ := authjwt.FromContext(r.Context())
	review, err := h.reviewSvc.SetHidden(r.Context(), r.PathValue("reviewId"), hidden, body.Reason, claims.UserID)
	if err != nil {
		h.respondReviewError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, review)
}

// ReviewMiddleware is what bootstrap wraps the review routes in.
type ReviewMiddleware struct {
	// Auth requires a valid bearer token (customer, manager or service).
	Auth func(http.Handler) http.Handler
	// Moderator requires product.review.moderate, after Auth.
	Moderator func(http.Handler) http.Handler
	// WriteLimit and UploadLimit budget a signed-in account; they run inside
	// Auth, so the caller is known. Nil means no limit.
	WriteLimit  func(http.Handler) http.Handler
	UploadLimit func(http.Handler) http.Handler
}

// RegisterReviewRoutes mounts the review API.
func (h *Handler) RegisterReviewRoutes(mux *http.ServeMux, m ReviewMiddleware) {
	id := func(next http.Handler) http.Handler { return next }
	if m.WriteLimit == nil {
		m.WriteLimit = id
	}
	if m.UploadLimit == nil {
		m.UploadLimit = id
	}
	auth := func(limit func(http.Handler) http.Handler, fn http.HandlerFunc) http.Handler {
		return m.Auth(limit(fn))
	}
	mux.HandleFunc("GET "+RouteProductReviews, h.ListProductReviews)
	mux.HandleFunc("GET "+RouteProductReviewPhotos, h.ListProductReviewPhotos)
	mux.Handle("POST "+RouteProductReviews, auth(m.WriteLimit, h.CreateReview))
	mux.Handle("POST "+RouteReviewPhotoUpload, auth(m.UploadLimit, h.UploadReviewPhoto))
	mux.Handle("GET "+RouteMyReviews, auth(id, h.MyReviews))
	mux.Handle("PATCH "+RouteReviewByID, auth(m.WriteLimit, h.UpdateReview))
	mux.Handle("DELETE "+RouteReviewByID, auth(m.WriteLimit, h.DeleteReview))
	mux.Handle("GET "+RouteReviews, m.Auth(m.Moderator(http.HandlerFunc(h.ModerateReviews))))
	mux.Handle("POST "+RouteReviewHide, m.Auth(m.Moderator(http.HandlerFunc(h.HideReview))))
}

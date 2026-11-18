package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

// Storefront banner routes. The live list is public (the storefront reads it
// without a token); everything else needs banner.manage. Single banners go
// under by-id/ so the paths cannot collide with /products/{id}/….
const (
	RouteBanners         = "/api/v1/products/banners"
	RouteBannersAll      = "/api/v1/products/banners/all"
	RouteBannerByID      = "/api/v1/products/banners/by-id/{id}"
	RouteBannerActive    = "/api/v1/products/banners/by-id/{id}/active"
	RouteBannerImages    = "/api/v1/products/banners/images"
	bannerUploadMaxBytes = service.BannerImageMaxBytes + 1<<20
)

func (h *Handler) WithBannerService(svc *service.BannerService) *Handler {
	h.bannerSvc = svc
	return h
}

// RegisterBannerRoutes mounts the public read; manage wraps the rest.
func (h *Handler) RegisterBannerRoutes(mux *http.ServeMux, manage func(http.Handler) http.Handler) {
	mux.HandleFunc("GET "+RouteBanners, h.LiveBanners)
	mux.Handle("GET "+RouteBannersAll, manage(http.HandlerFunc(h.ListBanners)))
	mux.Handle("POST "+RouteBanners, manage(http.HandlerFunc(h.CreateBanner)))
	mux.Handle("PUT "+RouteBannerByID, manage(http.HandlerFunc(h.UpdateBanner)))
	mux.Handle("PUT "+RouteBannerActive, manage(http.HandlerFunc(h.SetBannerActive)))
	mux.Handle("DELETE "+RouteBannerByID, manage(http.HandlerFunc(h.DeleteBanner)))
	mux.Handle("POST "+RouteBannerImages, manage(http.HandlerFunc(h.UploadBannerImage)))
}

func (h *Handler) bannersReady(w http.ResponseWriter) bool {
	if h.bannerSvc == nil {
		h.respondError(w, http.StatusServiceUnavailable, "banners are not configured")
		return false
	}
	return true
}

func bannerActor(r *http.Request) string {
	if claims, ok := authjwt.FromContext(r.Context()); ok {
		if claims.Email != "" {
			return claims.Email
		}
		return claims.UserID
	}
	return ""
}

// LiveBanners is what the storefront shows: `?placement=home_hero` (the
// default), active banners only, in display order.
func (h *Handler) LiveBanners(w http.ResponseWriter, r *http.Request) {
	if !h.bannersReady(w) {
		return
	}
	banners, err := h.bannerSvc.Live(r.Context(), domain.BannerPlacement(r.URL.Query().Get("placement")))
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	// A banner going up or down shows within a minute without every page
	// load reaching the database.
	w.Header().Set("Cache-Control", "public, max-age=60")
	h.respondJSON(w, http.StatusOK, map[string]any{"banners": banners})
}

func (h *Handler) ListBanners(w http.ResponseWriter, r *http.Request) {
	if !h.bannersReady(w) {
		return
	}
	banners, err := h.bannerSvc.List(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, map[string]any{"banners": banners})
}

func decodeBanner(r *http.Request) (domain.Banner, error) {
	var b domain.Banner
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&b); err != nil {
		return b, errors.New("invalid request body")
	}
	return b, nil
}

func (h *Handler) CreateBanner(w http.ResponseWriter, r *http.Request) {
	if !h.bannersReady(w) {
		return
	}
	b, err := decodeBanner(r)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.bannerSvc.Create(r.Context(), b, bannerActor(r))
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateBanner(w http.ResponseWriter, r *http.Request) {
	if !h.bannersReady(w) {
		return
	}
	b, err := decodeBanner(r)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.bannerSvc.Update(r.Context(), r.PathValue("id"), b, bannerActor(r))
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

// SetBannerActive takes `{ "active": true|false }`: show or take down.
func (h *Handler) SetBannerActive(w http.ResponseWriter, r *http.Request) {
	if !h.bannersReady(w) {
		return
	}
	var body struct {
		Active *bool `json:"active"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil || body.Active == nil {
		h.respondError(w, http.StatusBadRequest, "active is required")
		return
	}
	updated, err := h.bannerSvc.SetActive(r.Context(), r.PathValue("id"), *body.Active, bannerActor(r))
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteBanner(w http.ResponseWriter, r *http.Request) {
	if !h.bannersReady(w) {
		return
	}
	if err := h.bannerSvc.Delete(r.Context(), r.PathValue("id")); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UploadBannerImage takes a multipart `image` field and answers
// `{ "url": "…" }` for a banner's image_url or mobile_image_url.
func (h *Handler) UploadBannerImage(w http.ResponseWriter, r *http.Request) {
	if !h.bannersReady(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, bannerUploadMaxBytes)
	if err := r.ParseMultipartForm(bannerUploadMaxBytes); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid multipart form (at most 10 MB)")
		return
	}
	file, _, err := r.FormFile("image")
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "missing image field")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, service.BannerImageMaxBytes+1))
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "could not read the image")
		return
	}
	url, err := h.bannerSvc.UploadImage(r.Context(), data)
	if errors.Is(err, service.ErrBannerImagesUnavailable) {
		h.respondError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, map[string]string{"url": url})
}

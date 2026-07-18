package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/elug3/dupli1/product/pkg/authjwt"
	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

type Handler struct {
	svc          *service.ProductSearchService
	couponSvc    *service.CouponService
	inventorySvc *service.InventoryService
	catalogSvc   *service.CatalogService
	viewStore    ports.ProductViewStore
	guestCookie  GuestCookieConfig
	settings     settings.Response
}

type SearchResponse struct {
	Total   int         `json:"total"`
	Limit   int         `json:"limit"`
	Offset  int         `json:"offset"`
	Results interface{} `json:"results"`
}

type RecommendationsResponse struct {
	SeedID string           `json:"seedId"`
	Items  []domain.Product `json:"items"`
}

type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

var searchFilters = []string{"category", "brand", "color", "size", "material", "status"}

func NewHandler(svc *service.ProductSearchService, couponSvc *service.CouponService, inventorySvc *service.InventoryService, catalogSvc *service.CatalogService) *Handler {
	return &Handler{
		svc:         svc,
		couponSvc:   couponSvc,
		inventorySvc: inventorySvc,
		catalogSvc:  catalogSvc,
		guestCookie: defaultGuestCookieConfig(),
		settings:    settings.NewResponse("product"),
	}
}

// WithSettings sets the non-secret settings payload served by GET /settings.
func (h *Handler) WithSettings(s settings.Response) *Handler {
	h.settings = s
	return h
}

// WithViewStore enables unique guest PDP view recording.
func (h *Handler) WithViewStore(store ports.ProductViewStore) *Handler {
	h.viewStore = store
	return h
}

// WithGuestCookie configures the anonymous guest cookie used for unique views.
func (h *Handler) WithGuestCookie(cfg GuestCookieConfig) *Handler {
	h.guestCookie = cfg
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+RouteHealth, h.Health)
	mux.HandleFunc("GET "+RouteSettings, h.Settings)
	mux.HandleFunc("GET "+RouteProductRecommendations, h.PublicGetRecommendations)
	mux.HandleFunc("GET "+RoutePublicProduct, h.PublicGetProduct)
	mux.HandleFunc("GET "+RoutePublicVariant, h.PublicGetVariant)
	mux.HandleFunc("GET "+RoutePublicVariantBySkuID, h.PublicGetVariantBySkuID)
	mux.HandleFunc("POST "+RouteRedeemCoupon, h.RedeemCoupon)

	mux.HandleFunc("GET "+RouteInventoryHealth, h.Health)
	mux.HandleFunc("GET "+RouteInventorySettings, h.Settings)
	mux.HandleFunc("GET "+RouteInventoryItem, h.GetInventoryItem)
	mux.HandleFunc("GET "+RouteInventoryItemBySkuID, h.GetInventoryItemBySkuID)
}

// SearchProductsHandler returns an http.Handler for GET /api/v1/products.
func (h *Handler) SearchProductsHandler() http.Handler {
	return http.HandlerFunc(h.SearchProducts)
}

// UploadImageHandler returns an http.Handler for POST /api/v1/products/{id}/images.
// Images are stored on the default variant (legacy compatibility).
func (h *Handler) UploadImageHandler() http.Handler {
	return http.HandlerFunc(h.UploadProductImage)
}

// CreateVariantHandler returns an http.Handler for POST /api/v1/products/{id}/variants.
func (h *Handler) CreateVariantHandler() http.Handler {
	return http.HandlerFunc(h.CreateVariant)
}

// VariantBySKUHandler returns an http.Handler for PUT|DELETE /api/v1/products/{id}/variants/{sku}.
func (h *Handler) VariantBySKUHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			h.UpdateVariant(w, r)
		case http.MethodDelete:
			h.DeleteVariant(w, r)
		default:
			h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
}

// UploadVariantImageHandler returns an http.Handler for POST /api/v1/products/{id}/variants/{sku}/images.
func (h *Handler) UploadVariantImageHandler() http.Handler {
	return http.HandlerFunc(h.UploadVariantImage)
}

// CreateProductHandler returns an http.Handler for POST /api/v1/products.
func (h *Handler) CreateProductHandler() http.Handler {
	return http.HandlerFunc(h.CreateProduct)
}

// SingleProductHandler returns an http.Handler for PUT|DELETE /api/v1/products/{id}.
func (h *Handler) SingleProductHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			h.UpdateProduct(w, r)
		case http.MethodDelete:
			h.DeleteProduct(w, r)
		default:
			h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.respondJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.respondJSON(w, http.StatusOK, h.settings)
}

// SearchProducts lists/search products via query params.
// Public callers see active products only.
// Authenticated product managers see all statuses.
// Pagination: limit (default 50, max 100) and offset (default 0).
func (h *Handler) SearchProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r)
	public := true
	if claims, ok := authjwt.FromContext(r.Context()); ok && claims.HasPermission(permissions.ProductRead) {
		public = false
	} else {
		delete(filter, "status")
	}
	limit, offset := parseSearchPagination(r)
	filter["limit"] = strconv.Itoa(limit)
	filter["offset"] = strconv.Itoa(offset)

	results, total, err := h.svc.SearchProducts(filter, public)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	if results == nil {
		results = []domain.Product{}
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		Results: results,
	})
}

const (
	defaultSearchLimit = 50
	maxSearchLimit     = 100
)

func parseSearchPagination(r *http.Request) (limit, offset int) {
	limit = defaultSearchLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

func (h *Handler) PublicGetProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	product, err := h.svc.GetPublicProduct(id)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.recordProductView(w, r, product)
	h.respondJSON(w, http.StatusOK, product)
}

func (h *Handler) recordProductView(w http.ResponseWriter, r *http.Request, product *domain.Product) {
	if product == nil || h.viewStore == nil || !h.guestCookie.Enabled {
		return
	}
	guestID, minted := h.ensureGuestID(r)
	if minted {
		h.setGuestCookie(w, guestID)
	}
	inserted, err := h.viewStore.RecordUniqueView(guestID, product.ID)
	if err != nil {
		log.Printf("product view: record %s for guest: %v", product.ID, err)
		return
	}
	if inserted {
		product.ViewCount++
	}
}

func (h *Handler) PublicGetRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	limit := 8
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			h.respondError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}
	items, err := h.svc.Recommend(id, limit)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	if items == nil {
		items = []domain.Product{}
	}
	h.respondJSON(w, http.StatusOK, RecommendationsResponse{SeedID: id, Items: items})
}

func (h *Handler) PublicGetVariant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sku := r.PathValue("sku")
	if sku == "" {
		h.respondError(w, http.StatusBadRequest, "missing sku")
		return
	}
	variant, err := h.svc.GetPublicVariant(sku)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, variant)
}

func (h *Handler) PublicGetVariantBySkuID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	skuID := r.PathValue("skuId")
	if skuID == "" {
		h.respondError(w, http.StatusBadRequest, "missing skuId")
		return
	}
	variant, err := h.svc.GetPublicVariantBySkuID(skuID)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, variant)
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var p domain.Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if claims, ok := authjwt.FromContext(r.Context()); ok {
		p.CreatedBy = claims.UserID
	}
	created, err := h.svc.CreateProduct(p)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	var p domain.Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p.ID = id
	updated, err := h.svc.UpdateProduct(p)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	if err := h.svc.DeleteProduct(id); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCoupons(w http.ResponseWriter, r *http.Request) {
	coupons := h.couponSvc.List()
	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":   len(coupons),
		"results": coupons,
	})
}

func (h *Handler) CreateCoupon(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code        string  `json:"code"`
		Discount    float64 `json:"discount"`
		Description string  `json:"description"`
		Expires     string  `json:"expires"`
		Active      bool    `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	created, err := h.couponSvc.Create(domain.Coupon{
		Code:        body.Code,
		Discount:    body.Discount,
		Description: body.Description,
		Expires:     body.Expires,
		Active:      body.Active,
	})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateCoupon(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		h.respondError(w, http.StatusBadRequest, "missing coupon code")
		return
	}
	var body struct {
		Discount    *float64 `json:"discount"`
		Description *string  `json:"description"`
		Expires     *string  `json:"expires"`
		Active      *bool    `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	updated, err := h.couponSvc.Update(code, body.Discount, body.Description, body.Expires, body.Active)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteCoupon(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		h.respondError(w, http.StatusBadRequest, "missing coupon code")
		return
	}
	if err := h.couponSvc.Delete(code); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UploadProductImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	file, header, err := h.parseImageForm(r)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	product, err := h.svc.UploadImage(r.Context(), id, file, header.Size, contentType)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, product)
}

func (h *Handler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	var v domain.Variant
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	created, err := h.svc.CreateVariant(id, v)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateVariant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sku := r.PathValue("sku")
	if id == "" || sku == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id or sku")
		return
	}
	var v domain.Variant
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	updated, err := h.svc.UpdateVariant(id, sku, v)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteVariant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sku := r.PathValue("sku")
	if id == "" || sku == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id or sku")
		return
	}
	if err := h.svc.DeleteVariant(id, sku); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UploadVariantImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sku := r.PathValue("sku")
	if id == "" || sku == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id or sku")
		return
	}
	file, header, err := h.parseImageForm(r)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	variant, err := h.svc.UploadVariantImage(r.Context(), id, sku, file, header.Size, contentType)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, variant)
}

func (h *Handler) parseImageForm(r *http.Request) (multipart.File, *multipart.FileHeader, error) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		return nil, nil, fmt.Errorf("invalid multipart form")
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		return nil, nil, fmt.Errorf("missing image field")
	}
	return file, header, nil
}

func (h *Handler) RedeemCoupon(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Code == "" {
		h.respondError(w, http.StatusBadRequest, "code is required")
		return
	}
	coupon, ok := h.couponSvc.Redeem(body.Code)
	if !ok {
		h.respondError(w, http.StatusNotFound, "invalid coupon code")
		return
	}
	h.respondJSON(w, http.StatusOK, coupon)
}

func (h *Handler) extractFilters(r *http.Request) map[string]string {
	filter := make(map[string]string)
	for _, f := range searchFilters {
		if value := r.URL.Query().Get(f); value != "" {
			filter[f] = value
		}
	}
	if tags := collectTags(r); tags != "" {
		filter["tags"] = tags
	}
	return filter
}

func collectTags(r *http.Request) string {
	raw := r.URL.Query()["tags"]
	if len(raw) == 0 {
		return ""
	}
	var tags []string
	for _, entry := range raw {
		for _, part := range strings.Split(entry, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				tags = append(tags, part)
			}
		}
	}
	return strings.Join(tags, ",")
}

func (h *Handler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, ErrorResponse{Error: message, Code: status})
}

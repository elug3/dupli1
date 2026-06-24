package handler

import (
	"encoding/json"
	"net/http"

	"github.com/elug3/schick/product/pkg/domain"
	"github.com/elug3/schick/product/pkg/service"
)

type Handler struct {
	svc       *service.ProductSearchService
	couponSvc *service.CouponService
}

type SearchResponse struct {
	Total   int         `json:"total"`
	Results interface{} `json:"results"`
}

type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

var bagFilters = []string{"brand", "color", "material"}

func NewHandler(svc *service.ProductSearchService, couponSvc *service.CouponService) *Handler {
	return &Handler{svc: svc, couponSvc: couponSvc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("GET /api/products/bags", h.SearchBags)
	mux.HandleFunc("POST /api/coupons/redeem", h.RedeemCoupon)
}

// UploadImageHandler returns an http.Handler for PUT /api/products/{id}/image.
func (h *Handler) UploadImageHandler() http.Handler {
	return http.HandlerFunc(h.UploadProductImage)
}

// CreateProductHandler returns an http.Handler for POST /api/products.
func (h *Handler) CreateProductHandler() http.Handler {
	return http.HandlerFunc(h.CreateProduct)
}

// ListProductsHandler returns an http.Handler for GET /api/products.
func (h *Handler) ListProductsHandler() http.Handler {
	return http.HandlerFunc(h.ListProducts)
}

// SingleProductHandler returns an http.Handler for GET|PUT|DELETE /api/products/{id}.
func (h *Handler) SingleProductHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.GetProduct(w, r)
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
	h.respondJSON(w, http.StatusOK, HealthResponse{Status: "healthy"})
}

func (h *Handler) SearchBags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r)
	results, err := h.svc.SearchBags(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{Total: len(results), Results: results})
}

func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.svc.ListProducts()
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if products == nil {
		products = []domain.Product{}
	}
	h.respondJSON(w, http.StatusOK, products)
}

func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.respondError(w, http.StatusBadRequest, "missing product id")
		return
	}
	product, err := h.svc.GetProduct(id)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "product not found")
		return
	}
	h.respondJSON(w, http.StatusOK, product)
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
	created, err := h.svc.CreateProduct(p)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
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
		h.respondError(w, http.StatusInternalServerError, err.Error())
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
		h.respondError(w, http.StatusInternalServerError, err.Error())
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
		h.respondError(w, http.StatusConflict, err.Error())
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
		h.respondError(w, http.StatusNotFound, err.Error())
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
		h.respondError(w, http.StatusNotFound, err.Error())
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
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "missing image field")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	product, err := h.svc.UploadImage(r.Context(), id, file, header.Size, contentType)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, product)
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
	for _, f := range bagFilters {
		if value := r.URL.Query().Get(f); value != "" {
			filter[f] = value
		}
	}
	return filter
}

func (h *Handler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, ErrorResponse{Error: message, Code: status})
}

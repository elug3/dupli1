package handler

import (
	"encoding/json"
	"net/http"

	"github.com/schick/pkg/product/domain"
	"github.com/schick/pkg/product/service"
)

type Handler struct {
	svc *service.ProductSearchService
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

func NewHandler(svc *service.ProductSearchService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.Health)
	mux.HandleFunc("/api/products/bags", h.SearchBags)
}

// CreateProductHandler returns an http.Handler for POST /api/products.
// The caller is responsible for wrapping it with auth middleware.
func (h *Handler) CreateProductHandler() http.Handler {
	return http.HandlerFunc(h.CreateProduct)
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

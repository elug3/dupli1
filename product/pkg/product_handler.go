package product

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elug3/schick/product/pkg/domain"
)

type ProductSearchHandler struct {
	service *ProductSearchService
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

type CategoryResponse struct {
	Categories []string `json:"categories"`
}

var categories = []string{
	"consultations",
	"shoes",
	"outerwear",
	"bottoms",
	"bags",
	"clocks",
}

var categoryFilters = map[string][]string{
	"consultations": {"title", "status"},
	"shoes":         {"brand", "size", "color", "gender", "material"},
	"outerwear":     {"brand", "size", "color", "gender", "material"},
	"bottoms":       {"brand", "size", "color", "gender", "material"},
	"bags":          {"brand", "color", "material"},
	"clocks":        {"brand", "type", "material"},
}

func NewProductSearchHandler(service *ProductSearchService) *ProductSearchHandler {
	return &ProductSearchHandler{
		service: service,
	}
}

// RegisterRoutes registers all product handler routes
func (h *ProductSearchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/api/v1/products/health", h.Health)
	mux.HandleFunc("/api/v1/products/categories", h.GetCategories)
	mux.HandleFunc("/api/v1/products/filters", h.GetFilters)
	mux.HandleFunc("/api/v1/products/all", h.All)
	mux.HandleFunc("/api/v1/products/search", h.Search)
	mux.HandleFunc("/api/v1/products/consultations", h.SearchConsultations)
	mux.HandleFunc("/api/v1/products/shoes", h.SearchShoes)
	mux.HandleFunc("/api/v1/products/outerwear", h.SearchOuterwear)
	mux.HandleFunc("/api/v1/products/bottoms", h.SearchBottoms)
	mux.HandleFunc("/api/v1/products/bags", h.SearchBags)
	mux.HandleFunc("/api/v1/products/clocks", h.SearchClocks)
}

// Health returns server health status
func (h *ProductSearchHandler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.respondJSON(w, http.StatusOK, HealthResponse{Status: "healthy"})
}

func (h *ProductSearchHandler) All(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.service.SearchAll()
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, map[string]any{
		"total":   result.Total(),
		"results": result,
	})
}

// GetCategories returns available product categories
func (h *ProductSearchHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.respondJSON(w, http.StatusOK, CategoryResponse{Categories: categories})
}

// GetFilters returns available filters for a category
func (h *ProductSearchHandler) GetFilters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	category := r.URL.Query().Get("category")
	if category == "" {
		h.respondError(w, http.StatusBadRequest, "category query parameter is required")
		return
	}

	filters, ok := categoryFilters[category]
	if !ok {
		h.respondError(w, http.StatusNotFound, fmt.Sprintf("unknown category: %s", category))
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"category": category,
		"filters":  filters,
	})
}

// Search handles generic product search with category parameter
func (h *ProductSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	category := r.URL.Query().Get("category")
	if category == "" {
		h.respondError(w, http.StatusBadRequest, "category query parameter is required")
		return
	}

	// Extract filter parameters
	filter := h.extractFilters(r, category)

	results, err := h.service.Search(category, filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Count results
	var count int
	switch v := results.(type) {
	case []domain.Consultation:
		count = len(v)
	case []domain.Shoes:
		count = len(v)
	case []domain.Outerwear:
		count = len(v)
	case []domain.Bottoms:
		count = len(v)
	case []domain.Bag:
		count = len(v)
	case []domain.Clock:
		count = len(v)
	}

	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   count,
		Results: results,
	})
}

// SearchConsultations handles consultation search
func (h *ProductSearchHandler) SearchConsultations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := h.extractFilters(r, "consultations")
	results, err := h.service.SearchConsultations(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   len(results),
		Results: results,
	})
}

// SearchShoes handles shoes search
func (h *ProductSearchHandler) SearchShoes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := h.extractFilters(r, "shoes")
	results, err := h.service.SearchShoes(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   len(results),
		Results: results,
	})
}

// SearchOuterwear handles outerwear search
func (h *ProductSearchHandler) SearchOuterwear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := h.extractFilters(r, "outerwear")
	results, err := h.service.SearchOuterwear(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   len(results),
		Results: results,
	})
}

// SearchBottoms handles bottoms search
func (h *ProductSearchHandler) SearchBottoms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := h.extractFilters(r, "bottoms")
	results, err := h.service.SearchBottoms(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   len(results),
		Results: results,
	})
}

// SearchBags handles bags search
func (h *ProductSearchHandler) SearchBags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := h.extractFilters(r, "bags")
	results, err := h.service.SearchBags(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   len(results),
		Results: results,
	})
}

// SearchClocks handles clocks search
func (h *ProductSearchHandler) SearchClocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := h.extractFilters(r, "clocks")
	results, err := h.service.SearchClocks(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, SearchResponse{
		Total:   len(results),
		Results: results,
	})
}

// extractFilters extracts query parameters as filters
func (h *ProductSearchHandler) extractFilters(r *http.Request, category string) map[string]string {
	filter := make(map[string]string)
	allowedFilters := categoryFilters[category]

	for _, f := range allowedFilters {
		if value := r.URL.Query().Get(f); value != "" {
			filter[f] = value
		}
	}

	return filter
}

// respondJSON writes a JSON response
func (h *ProductSearchHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error response
func (h *ProductSearchHandler) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: message,
		Code:  status,
	})
}

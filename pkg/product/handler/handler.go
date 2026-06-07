package handler

import (
	"encoding/json"
	"fmt"
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

func NewHandler(svc *service.ProductSearchService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.Health)
	mux.HandleFunc("/api/categories", h.GetCategories)
	mux.HandleFunc("/api/filters", h.GetFilters)
	mux.HandleFunc("/api/products/search", h.Search)
	mux.HandleFunc("/api/products/consultations", h.SearchConsultations)
	mux.HandleFunc("/api/products/shoes", h.SearchShoes)
	mux.HandleFunc("/api/products/outerwear", h.SearchOuterwear)
	mux.HandleFunc("/api/products/bottoms", h.SearchBottoms)
	mux.HandleFunc("/api/products/bags", h.SearchBags)
	mux.HandleFunc("/api/products/clocks", h.SearchClocks)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.respondJSON(w, http.StatusOK, HealthResponse{Status: "healthy"})
}

func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.respondJSON(w, http.StatusOK, CategoryResponse{Categories: categories})
}

func (h *Handler) GetFilters(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	category := r.URL.Query().Get("category")
	if category == "" {
		h.respondError(w, http.StatusBadRequest, "category query parameter is required")
		return
	}

	filter := h.extractFilters(r, category)
	results, err := h.svc.Search(category, filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

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

	h.respondJSON(w, http.StatusOK, SearchResponse{Total: count, Results: results})
}

func (h *Handler) SearchConsultations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r, "consultations")
	results, err := h.svc.SearchConsultations(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{Total: len(results), Results: results})
}

func (h *Handler) SearchShoes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r, "shoes")
	results, err := h.svc.SearchShoes(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{Total: len(results), Results: results})
}

func (h *Handler) SearchOuterwear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r, "outerwear")
	results, err := h.svc.SearchOuterwear(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{Total: len(results), Results: results})
}

func (h *Handler) SearchBottoms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r, "bottoms")
	results, err := h.svc.SearchBottoms(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{Total: len(results), Results: results})
}

func (h *Handler) SearchBags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r, "bags")
	results, err := h.svc.SearchBags(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{Total: len(results), Results: results})
}

func (h *Handler) SearchClocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	filter := h.extractFilters(r, "clocks")
	results, err := h.svc.SearchClocks(filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respondJSON(w, http.StatusOK, SearchResponse{Total: len(results), Results: results})
}

func (h *Handler) extractFilters(r *http.Request, category string) map[string]string {
	filter := make(map[string]string)
	for _, f := range categoryFilters[category] {
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

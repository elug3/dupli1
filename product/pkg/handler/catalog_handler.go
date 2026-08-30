package handler

import (
	"encoding/json"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/domain"
)

type nameBody struct {
	Name string `json:"name"`
}

type codeNameBody struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// --- Brands ---

func (h *Handler) ListBrands(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListBrands(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateBrand(w http.ResponseWriter, r *http.Request) {
	var body codeNameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	created, err := h.catalogSvc.CreateBrand(r.Context(), domain.Brand{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateBrand(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var body nameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated, err := h.catalogSvc.UpdateBrandName(r.Context(), code, body.Name)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteBrand(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteBrand(r.Context(), r.PathValue("code")); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Styles ---

func (h *Handler) ListStyles(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListStyles(r.Context(), r.PathValue("code"))
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateStyle(w http.ResponseWriter, r *http.Request) {
	brandCode := r.PathValue("code")
	var body codeNameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	created, err := h.catalogSvc.CreateStyle(r.Context(), domain.Style{
		BrandCode: brandCode, Code: body.Code, Name: body.Name,
	})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateStyle(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated, err := h.catalogSvc.UpdateStyleName(r.Context(), r.PathValue("code"), r.PathValue("styleCode"), body.Name)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteStyle(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteStyle(r.Context(), r.PathValue("code"), r.PathValue("styleCode")); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Colors ---

func (h *Handler) ListColors(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListColors(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateColor(w http.ResponseWriter, r *http.Request) {
	var body codeNameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	created, err := h.catalogSvc.CreateColor(r.Context(), domain.Color{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateColor(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated, err := h.catalogSvc.UpdateColorName(r.Context(), r.PathValue("code"), body.Name)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteColor(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteColor(r.Context(), r.PathValue("code")); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Sizes ---

func (h *Handler) ListSizes(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListSizes(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateSize(w http.ResponseWriter, r *http.Request) {
	var body codeNameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	created, err := h.catalogSvc.CreateSize(r.Context(), domain.Size{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateSize(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated, err := h.catalogSvc.UpdateSizeName(r.Context(), r.PathValue("code"), body.Name)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteSize(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteSize(r.Context(), r.PathValue("code")); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Editions ---

func (h *Handler) ListEditions(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListEditions(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateEdition(w http.ResponseWriter, r *http.Request) {
	var body codeNameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	created, err := h.catalogSvc.CreateEdition(r.Context(), domain.Edition{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateEdition(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated, err := h.catalogSvc.UpdateEditionName(r.Context(), r.PathValue("code"), body.Name)
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteEdition(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteEdition(r.Context(), r.PathValue("code")); err != nil {
		h.respondServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Bag merchandising taxonomy ---

func (h *Handler) GetMasterCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := h.catalogSvc.MasterCatalog(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, catalog)
}

func (h *Handler) ListSubCategories(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListSubCategories(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

func (h *Handler) ListBagStyles(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListBagStyles(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

func (h *Handler) ListTargets(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListTargets(r.Context())
	if err != nil {
		h.respondServiceError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, list)
}

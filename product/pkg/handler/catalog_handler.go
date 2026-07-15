package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elug3/dupli1/product/pkg/domain"
)

func (h *Handler) respondCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrMasterNotFound):
		h.respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrMasterExists):
		h.respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrMasterInUse):
		h.respondError(w, http.StatusConflict, err.Error())
	default:
		h.respondError(w, http.StatusBadRequest, err.Error())
	}
}

type nameBody struct {
	Name string `json:"name"`
}

type codeNameBody struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// --- Brands ---

func (h *Handler) ListBrands(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListBrands()
	if err != nil {
		h.respondCatalogError(w, err)
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
	created, err := h.catalogSvc.CreateBrand(domain.Brand{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondCatalogError(w, err)
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
	updated, err := h.catalogSvc.UpdateBrandName(code, body.Name)
	if err != nil {
		h.respondCatalogError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteBrand(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteBrand(r.PathValue("code")); err != nil {
		h.respondCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Styles ---

func (h *Handler) ListStyles(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListStyles(r.PathValue("code"))
	if err != nil {
		h.respondCatalogError(w, err)
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
	created, err := h.catalogSvc.CreateStyle(domain.Style{
		BrandCode: brandCode, Code: body.Code, Name: body.Name,
	})
	if err != nil {
		h.respondCatalogError(w, err)
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
	updated, err := h.catalogSvc.UpdateStyleName(r.PathValue("code"), r.PathValue("styleCode"), body.Name)
	if err != nil {
		h.respondCatalogError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteStyle(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteStyle(r.PathValue("code"), r.PathValue("styleCode")); err != nil {
		h.respondCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Colors ---

func (h *Handler) ListColors(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListColors()
	if err != nil {
		h.respondCatalogError(w, err)
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
	created, err := h.catalogSvc.CreateColor(domain.Color{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondCatalogError(w, err)
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
	updated, err := h.catalogSvc.UpdateColorName(r.PathValue("code"), body.Name)
	if err != nil {
		h.respondCatalogError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteColor(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteColor(r.PathValue("code")); err != nil {
		h.respondCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Sizes ---

func (h *Handler) ListSizes(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListSizes()
	if err != nil {
		h.respondCatalogError(w, err)
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
	created, err := h.catalogSvc.CreateSize(domain.Size{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondCatalogError(w, err)
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
	updated, err := h.catalogSvc.UpdateSizeName(r.PathValue("code"), body.Name)
	if err != nil {
		h.respondCatalogError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteSize(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteSize(r.PathValue("code")); err != nil {
		h.respondCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Editions ---

func (h *Handler) ListEditions(w http.ResponseWriter, r *http.Request) {
	list, err := h.catalogSvc.ListEditions()
	if err != nil {
		h.respondCatalogError(w, err)
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
	created, err := h.catalogSvc.CreateEdition(domain.Edition{Code: body.Code, Name: body.Name})
	if err != nil {
		h.respondCatalogError(w, err)
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
	updated, err := h.catalogSvc.UpdateEditionName(r.PathValue("code"), body.Name)
	if err != nil {
		h.respondCatalogError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteEdition(w http.ResponseWriter, r *http.Request) {
	if err := h.catalogSvc.DeleteEdition(r.PathValue("code")); err != nil {
		h.respondCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

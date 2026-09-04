// Package handler implements the profile service's HTTP surface using the
// stdlib net/http.ServeMux (method+pattern routing, Go 1.22+), not gin —
// auth's copy of this handler used gin because it lived inside auth's
// gin-based router; profile is a standalone service and follows the
// stdlib convention used by cart/order/payment/product/notification.
package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/profile/pkg/apperrors"
	"github.com/elug3/dupli1/profile/pkg/domain"
	"github.com/elug3/dupli1/profile/pkg/ports"
	"github.com/elug3/dupli1/profile/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/authmiddleware"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

type Handler struct {
	svc          *service.Service
	jwtValidator authjwt.AccessTokenValidator
	settings     settings.Response
}

func New(svc *service.Service, jwtValidator authjwt.AccessTokenValidator) *Handler {
	return &Handler{
		svc:          svc,
		jwtValidator: jwtValidator,
		settings:     settings.NewResponse("profile"),
	}
}

// WithSettings sets the non-secret settings payload served by GET /settings.
func (h *Handler) WithSettings(s settings.Response) *Handler {
	h.settings = s
	return h
}

// RegisterRoutes registers the canonical /api/v1/profile/me/... routes plus
// legacy /api/v1/auth/me/... aliases pointing at the same handlers, so the
// gateway can be switched over to this service one route at a time without
// breaking clients still calling the old auth-hosted paths. Remove the
// legacy aliases once nginx routes exclusively to /api/v1/profile.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /api/v1/profile/health", h.health)
	mux.HandleFunc("GET /settings", h.settingsHandler)
	mux.HandleFunc("GET /api/v1/profile/settings", h.settingsHandler)

	for _, prefix := range []string{"/api/v1/profile/me", "/api/v1/auth/me"} {
		mux.HandleFunc("GET "+prefix+"/profile", h.requireAuth(h.getProfile))
		mux.HandleFunc("PATCH "+prefix+"/profile", h.requireAuth(h.patchProfile))
		mux.HandleFunc("GET "+prefix+"/addresses", h.requireAuth(h.listAddresses))
		mux.HandleFunc("POST "+prefix+"/addresses", h.requireAuth(h.createAddress))
		mux.HandleFunc("GET "+prefix+"/addresses/{id}", h.requireAuth(h.getAddress))
		mux.HandleFunc("PATCH "+prefix+"/addresses/{id}", h.requireAuth(h.patchAddress))
		mux.HandleFunc("DELETE "+prefix+"/addresses/{id}", h.requireAuth(h.deleteAddress))
		mux.HandleFunc("POST "+prefix+"/addresses/{id}/default", h.requireAuth(h.setDefaultAddress))
	}
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) settingsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, h.settings)
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return authmiddleware.RequireAuth(h.jwtValidator, respondError)(next)
}

func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	view, err := h.svc.GetProfileView(r.Context(), claims.UserID)
	if err != nil {
		respondServiceError(w, "get_profile_error", err)
		return
	}
	respondJSON(w, http.StatusOK, view)
}

func (h *Handler) patchProfile(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	var patch service.ProfilePatch
	if err := decodeJSON(r, &patch); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	view, err := h.svc.PatchProfile(r.Context(), claims.UserID, patch)
	if err != nil {
		respondServiceError(w, "patch_profile_error", err)
		return
	}
	respondJSON(w, http.StatusOK, view)
}

func (h *Handler) listAddresses(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	addresses, err := h.svc.ListAddresses(r.Context(), claims.UserID)
	if err != nil {
		respondServiceError(w, "list_addresses_error", err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"addresses": addresses})
}

func (h *Handler) createAddress(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	var input service.AddressInput
	if err := decodeJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	address, err := h.svc.CreateAddress(r.Context(), claims.UserID, input)
	if err != nil {
		respondServiceError(w, "create_address_error", err)
		return
	}
	respondJSON(w, http.StatusCreated, address)
}

func (h *Handler) getAddress(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	addressID := strings.TrimSpace(r.PathValue("id"))
	address, err := h.svc.GetAddress(r.Context(), claims.UserID, addressID)
	if err != nil {
		respondServiceError(w, "get_address_error", err)
		return
	}
	respondJSON(w, http.StatusOK, address)
}

func (h *Handler) patchAddress(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	addressID := strings.TrimSpace(r.PathValue("id"))
	var input service.AddressInput
	if err := decodeJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	address, err := h.svc.PatchAddress(r.Context(), claims.UserID, addressID, input)
	if err != nil {
		respondServiceError(w, "patch_address_error", err)
		return
	}
	respondJSON(w, http.StatusOK, address)
}

func (h *Handler) deleteAddress(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	addressID := strings.TrimSpace(r.PathValue("id"))
	if err := h.svc.DeleteAddress(r.Context(), claims.UserID, addressID); err != nil {
		respondServiceError(w, "delete_address_error", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setDefaultAddress(w http.ResponseWriter, r *http.Request) {
	claims, _ := authjwt.FromContext(r.Context())
	addressID := strings.TrimSpace(r.PathValue("id"))
	address, err := h.svc.SetDefaultAddress(r.Context(), claims.UserID, addressID)
	if err != nil {
		respondServiceError(w, "set_default_address_error", err)
		return
	}
	respondJSON(w, http.StatusOK, address)
}

func respondServiceError(w http.ResponseWriter, event string, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidProfile), errors.Is(err, domain.ErrInvalidAddress):
		respondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ports.ErrAddressNotFound):
		respondError(w, http.StatusNotFound, "address not found")
	case errors.Is(err, apperrors.ErrAddressLimitReached):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("profile: internal error event=%s: %v", event, err)
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]any{
		"error": message,
		"code":  status,
	})
}

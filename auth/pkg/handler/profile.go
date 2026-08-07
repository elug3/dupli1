package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/auth/pkg/service"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetProfile(c *gin.Context) {
	caller := callerFromContext(c)
	view, err := h.svc.GetProfileView(c.Request.Context(), caller.ID)
	if err != nil {
		h.respondProfileError(c, "get_profile_error", err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *Handler) PatchProfile(c *gin.Context) {
	caller := callerFromContext(c)
	var patch service.ProfilePatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	view, err := h.svc.PatchProfile(c.Request.Context(), caller.ID, patch)
	if err != nil {
		h.respondProfileError(c, "patch_profile_error", err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *Handler) ListAddresses(c *gin.Context) {
	caller := callerFromContext(c)
	addresses, err := h.svc.ListAddresses(c.Request.Context(), caller.ID)
	if err != nil {
		h.respondProfileError(c, "list_addresses_error", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"addresses": addresses})
}

func (h *Handler) CreateAddress(c *gin.Context) {
	caller := callerFromContext(c)
	var input service.AddressInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	address, err := h.svc.CreateAddress(c.Request.Context(), caller.ID, input)
	if err != nil {
		h.respondProfileError(c, "create_address_error", err)
		return
	}
	c.JSON(http.StatusCreated, address)
}

func (h *Handler) GetAddress(c *gin.Context) {
	caller := callerFromContext(c)
	addressID := strings.TrimSpace(c.Param("id"))
	address, err := h.svc.GetAddress(c.Request.Context(), caller.ID, addressID)
	if err != nil {
		h.respondProfileError(c, "get_address_error", err)
		return
	}
	c.JSON(http.StatusOK, address)
}

func (h *Handler) PatchAddress(c *gin.Context) {
	caller := callerFromContext(c)
	addressID := strings.TrimSpace(c.Param("id"))
	var input service.AddressInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	address, err := h.svc.PatchAddress(c.Request.Context(), caller.ID, addressID, input)
	if err != nil {
		h.respondProfileError(c, "patch_address_error", err)
		return
	}
	c.JSON(http.StatusOK, address)
}

func (h *Handler) DeleteAddress(c *gin.Context) {
	caller := callerFromContext(c)
	addressID := strings.TrimSpace(c.Param("id"))
	if err := h.svc.DeleteAddress(c.Request.Context(), caller.ID, addressID); err != nil {
		h.respondProfileError(c, "delete_address_error", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) SetDefaultAddress(c *gin.Context) {
	caller := callerFromContext(c)
	addressID := strings.TrimSpace(c.Param("id"))
	address, err := h.svc.SetDefaultAddress(c.Request.Context(), caller.ID, addressID)
	if err != nil {
		h.respondProfileError(c, "set_default_address_error", err)
		return
	}
	c.JSON(http.StatusOK, address)
}

func (h *Handler) respondProfileError(c *gin.Context, event string, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidProfile),
		errors.Is(err, domain.ErrInvalidAddress):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ports.ErrAddressNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
	case errors.Is(err, autherrors.ErrAddressLimitReached):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		h.respondInternalError(c, event, err)
	}
}

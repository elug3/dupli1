package handler

import (
	"net/http"

	"github.com/elug3/schick/pkg/auth/service"
	"github.com/gin-gonic/gin"
)

// Handler holds service dependencies for HTTP handlers.
type Handler struct {
	svc *service.Service
}

// NewHandler creates a new Handler.
func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Login handles user login and returns a refresh token in JSON.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"refresh_token": token})
}

// Logout invalidates a user's session.
func (h *Handler) Logout(c *gin.Context) {
	// TODO: extract userID from context/session and call service.Logout
	c.Status(http.StatusNoContent)
}

// Refresh exchanges a refresh token for a new access token.
func (h *Handler) Refresh(c *gin.Context) {
	var payload struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newToken, err := h.svc.Refresh(c.Request.Context(), payload.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": newToken})
}

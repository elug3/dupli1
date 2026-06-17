package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/elug3/schick/pkg/auth/autherrors"
	"github.com/elug3/schick/pkg/auth/domain"
	"github.com/elug3/schick/pkg/auth/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler holds service dependencies for HTTP handlers.
type Handler struct {
	svc *service.Service
}

// NewHandler creates a new Handler.
func NewHandler(svc *service.Service) *Handler {
	h := &Handler{svc: svc}
	return h
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type meResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user_id": user.ID})
}

// Login handles user login and returns a refresh token in JSON.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	})
}

// Logout invalidates a user's session.
func (h *Handler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Me returns the currently authenticated user.
func (h *Handler) Me(c *gin.Context) {
	token, ok := bearerToken(c.GetHeader("Authorization"))
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
		return
	}

	user, err := h.svc.Me(c.Request.Context(), token)
	if err != nil {
		status := http.StatusUnauthorized
		message := "invalid token"
		if errors.Is(err, autherrors.ErrUserNotFound) {
			status = http.StatusNotFound
			message = "user not found"
		}
		c.JSON(status, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, meResponse{
		ID:    user.ID.String(),
		Email: user.Email,
		Role:  user.Role,
	})
}

// Refresh exchanges a refresh token for a new access token.
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	})
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}

	return strings.TrimSpace(token), true
}

// ── Admin user management ─────────────────────────────────────────────────────

type adminUserResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

func userToResponse(u *domain.User) adminUserResponse {
	return adminUserResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
	}
}

// RequireAdmin is a Gin middleware that validates a Bearer token and checks that
// the caller has the "owner" or "admin" role.
func (h *Handler) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		user, err := h.svc.Me(c.Request.Context(), token)
		if err != nil || user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if user.Role != "owner" && user.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Set("currentUser", user)
		c.Next()
	}
}

// ListUsers returns all users.
func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := make([]adminUserResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, userToResponse(u))
	}
	c.JSON(http.StatusOK, resp)
}

// GetUser returns a single user by id.
func (h *Handler) GetUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, u := range users {
		if u.ID == id {
			c.JSON(http.StatusOK, userToResponse(u))
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
}

type createUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role"`
}

// CreateUser creates a new user (admin-protected, role selectable).
func (h *Handler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	validRoles := map[string]bool{"user": true, "admin": true, "owner": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	user, err := h.svc.CreateUser(c.Request.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, userToResponse(user))
}

type updateRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// UpdateUserRole changes a user's role.
func (h *Handler) UpdateUserRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validRoles := map[string]bool{"user": true, "admin": true, "owner": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	updated, err := h.svc.UpdateUserRole(c.Request.Context(), id, req.Role)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userToResponse(updated))
}

// DeleteUser removes a user by id.
func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := h.svc.DeleteUser(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

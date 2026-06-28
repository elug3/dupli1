package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elug3/schick/auth/pkg/autherrors"
	"github.com/elug3/schick/auth/pkg/domain"
	"github.com/elug3/schick/auth/pkg/service"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type userResponse struct {
	ID                  string     `json:"user_id"`
	Email               string     `json:"email"`
	Roles               []string   `json:"roles"`
	IsActive            bool       `json:"is_active"`
	LockedAt            *time.Time `json:"locked_at,omitempty"`
	FailedLoginAttempts int        `json:"failed_login_attempts"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:                  u.ID,
		Email:               u.Email,
		Roles:               u.Roles,
		IsActive:            u.IsActive,
		LockedAt:            u.LockedAt,
		FailedLoginAttempts: u.FailedLoginAttempts,
	}
}

// Handler holds service dependencies for HTTP handlers.
// Access control is enforced via RequireAuth / RequireRole middleware in the router.
type Handler struct {
	svc    *service.Service
	logger zerolog.Logger
}

// NewHandler creates a new Handler.
func NewHandler(svc *service.Service, logger zerolog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Login handles user login and returns a refresh token.
func (h *Handler) Login(c *gin.Context) {
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn().
			Str("event", "login_bad_request").
			Str("ip", ip).
			Str("error", err.Error()).
			Msg("login: invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("login: parse request: %w", err).Error()})
		return
	}

	token, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, autherrors.ErrInvalidCredentials) {
			h.logger.Warn().
				Str("event", "login_failed").
				Str("email", req.Email).
				Str("ip", ip).
				Str("user_agent", ua).
				Msg("login failed: invalid credentials")
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Errorf("login: %w", err).Error()})
		} else if errors.Is(err, autherrors.ErrAccountLocked) {
			h.logger.Warn().
				Str("event", "login_locked").
				Str("email", req.Email).
				Str("ip", ip).
				Msg("login failed: account locked")
			c.JSON(http.StatusForbidden, gin.H{"error": fmt.Errorf("login: %w", err).Error()})
		} else {
			h.logger.Error().
				Str("event", "login_error").
				Str("email", req.Email).
				Str("ip", ip).
				Err(err).
				Msg("login failed: internal error")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("login: %w", err).Error()})
		}
		return
	}

	h.logger.Info().
		Str("event", "login_success").
		Str("email", req.Email).
		Str("ip", ip).
		Str("user_agent", ua).
		Msg("login successful")

	c.JSON(http.StatusOK, gin.H{"refresh_token": token})
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// Register creates a new user account.
func (h *Handler) Register(c *gin.Context) {
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")

	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn().
			Str("event", "register_bad_request").
			Str("ip", ip).
			Str("error", err.Error()).
			Msg("register: invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("register: parse request: %w", err).Error()})
		return
	}

	u, err := h.svc.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserAlreadyExists) {
			h.logger.Warn().
				Str("event", "register_conflict").
				Str("email", req.Email).
				Str("ip", ip).
				Msg("register failed: user already exists")
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Errorf("register: %w", err).Error()})
		} else if errors.Is(err, autherrors.ErrInvalidEmail) || errors.Is(err, autherrors.ErrWeakPassword) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Errorf("register: %w", err).Error()})
		} else {
			h.logger.Error().
				Str("event", "register_error").
				Str("email", req.Email).
				Str("ip", ip).
				Err(err).
				Msg("register failed: internal error")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("register: %w", err).Error()})
		}
		return
	}

	h.logger.Info().
		Str("event", "register_success").
		Str("user_id", u.ID).
		Str("email", u.Email).
		Str("ip", ip).
		Str("user_agent", ua).
		Msg("user registered successfully")

	c.JSON(http.StatusCreated, gin.H{"user_id": u.ID})
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Logout revokes a refresh token. The refresh token is provided in the request body.
func (h *Handler) Logout(c *gin.Context) {
	ip := c.ClientIP()

	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("logout: parse request: %w", err).Error()})
		return
	}

	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		h.logger.Error().Str("event", "logout_error").Str("ip", ip).Err(err).Msg("logout: internal error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("logout: %w", err).Error()})
		return
	}

	h.logger.Info().Str("event", "logout").Str("ip", ip).Msg("logout successful")
	c.Status(http.StatusNoContent)
}

// Me returns the authenticated user's profile. Requires RequireAuth middleware.
func (h *Handler) Me(c *gin.Context) {
	c.JSON(http.StatusOK, toUserResponse(callerFromContext(c)))
}

// ListUsers returns all users. Requires RequireAuth + RequireRole("admin") middleware.
func (h *Handler) ListUsers(c *gin.Context) {
	ip := c.ClientIP()

	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		h.logger.Error().Str("event", "list_users_error").Str("ip", ip).Err(err).Msg("list users: internal error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("list users: %w", err).Error()})
		return
	}

	out := make([]userResponse, len(users))
	for i, u := range users {
		out[i] = toUserResponse(u)
	}

	c.JSON(http.StatusOK, gin.H{"users": out})
}

// Refresh exchanges a refresh token for a new short-lived access token.
func (h *Handler) Refresh(c *gin.Context) {
	ip := c.ClientIP()

	var payload struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		h.logger.Warn().
			Str("event", "refresh_bad_request").
			Str("ip", ip).
			Str("error", err.Error()).
			Msg("refresh: invalid request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("refresh: parse request: %w", err).Error()})
		return
	}

	newToken, err := h.svc.Refresh(c.Request.Context(), payload.RefreshToken)
	if err != nil {
		if errors.Is(err, autherrors.ErrInvalidToken) || errors.Is(err, autherrors.ErrTokenExpired) {
			h.logger.Warn().
				Str("event", "refresh_invalid_token").
				Str("ip", ip).
				Err(err).
				Msg("refresh failed: invalid or expired token")
		} else {
			h.logger.Error().
				Str("event", "refresh_error").
				Str("ip", ip).
				Err(err).
				Msg("refresh failed: internal error")
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Errorf("refresh: %w", err).Error()})
		return
	}

	h.logger.Info().
		Str("event", "refresh_success").
		Str("ip", ip).
		Msg("token refreshed successfully")

	c.JSON(http.StatusOK, gin.H{"token": newToken})
}

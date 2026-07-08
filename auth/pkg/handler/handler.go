package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type userResponse struct {
	ID                  string     `json:"user_id"`
	Email               string     `json:"email"`
	AccountType         string     `json:"account_type"`
	Permissions         []string   `json:"permissions"`
	IsActive            bool       `json:"is_active"`
	LockedAt            *time.Time `json:"locked_at,omitempty"`
	FailedLoginAttempts int        `json:"failed_login_attempts"`
}

func toUserResponse(u *domain.User) userResponse {
	perms := u.Permissions
	if perms == nil {
		perms = []string{}
	}
	return userResponse{
		ID:                  u.ID,
		Email:               u.Email,
		AccountType:         u.AccountType,
		Permissions:         perms,
		IsActive:            u.IsActive,
		LockedAt:            u.LockedAt,
		FailedLoginAttempts: u.FailedLoginAttempts,
	}
}

// Handler holds service dependencies for HTTP handlers.
// Access control is enforced via RequireAuth / RequirePermission middleware in the router.
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
		} else if errors.Is(err, autherrors.ErrAccountDeactivated) {
			h.logger.Warn().
				Str("event", "login_deactivated").
				Str("email", req.Email).
				Str("ip", ip).
				Msg("login failed: account deactivated")
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
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	AccountType string `json:"account_type"`
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

	accountType := req.AccountType
	if accountType == "" {
		accountType = domain.DefaultAccountType
	}
	caller := callerFromContext(c)
	if caller == nil || !domain.CanRegister(caller, accountType, nil) {
		c.JSON(http.StatusForbidden, gin.H{"error": "management forbidden: cannot register this account type"})
		return
	}
	if domain.ClassFromNewUser(accountType, nil) == domain.ClassOwner {
		hasOwner, err := h.svc.HasOwner(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "register: internal error"})
			return
		}
		if hasOwner {
			c.JSON(http.StatusForbidden, gin.H{"error": "owner account already exists"})
			return
		}
	}

	u, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, accountType)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserAlreadyExists) {
			h.logger.Warn().
				Str("event", "register_conflict").
				Str("email", req.Email).
				Str("ip", ip).
				Msg("register failed: user already exists")
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Errorf("register: %w", err).Error()})
		} else if errors.Is(err, autherrors.ErrInvalidEmail) || errors.Is(err, autherrors.ErrWeakPassword) || errors.Is(err, autherrors.ErrInvalidAccountType) {
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

// ListUsers returns all users. Requires user.read permission.
func (h *Handler) ListUsers(c *gin.Context) {
	ip := c.ClientIP()

	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		h.logger.Error().Str("event", "list_users_error").Str("ip", ip).Err(err).Msg("list users: internal error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("list users: %w", err).Error()})
		return
	}

	caller := callerFromContext(c)
	filtered := make([]*domain.User, 0, len(users))
	for _, u := range users {
		if domain.CanManageUser(caller, u) {
			filtered = append(filtered, u)
		}
	}

	out := make([]userResponse, len(filtered))
	for i, u := range filtered {
		out[i] = toUserResponse(u)
	}

	c.JSON(http.StatusOK, gin.H{"users": out})
}

// SetUserPermissions replaces the permission list for a user.
func (h *Handler) SetUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	var body struct {
		Permissions []string `json:"permissions"`
		AccountType string   `json:"account_type"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("set permissions: parse request: %w", err).Error()})
		return
	}
	perms := body.Permissions
	if len(perms) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "permissions is required"})
		return
	}

	caller := callerFromContext(c)
	target, err := h.svc.FindUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("set permissions: %w", err).Error()})
		}
		return
	}
	if !domain.CanAssignPermissions(caller, target, perms, body.AccountType) {
		c.JSON(http.StatusForbidden, gin.H{"error": autherrors.ErrManagementForbidden.Error()})
		return
	}
	if permissions.Has(perms, permissions.All) {
		hasOwner, err := h.svc.HasOwner(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "set permissions: internal error"})
			return
		}
		if hasOwner && !permissions.Has(target.Permissions, permissions.All) {
			c.JSON(http.StatusForbidden, gin.H{"error": "owner account already exists"})
			return
		}
	}

	u, err := h.svc.SetUserPermissions(c.Request.Context(), userID, perms, body.AccountType)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else if errors.Is(err, autherrors.ErrInvalidAccountType) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Errorf("set permissions: %w", err).Error()})
		} else if errors.Is(err, autherrors.ErrInvalidPermission) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Errorf("set permissions: %w", err).Error()})
		} else {
			h.logger.Error().Str("event", "set_permissions_error").Str("user_id", userID).Err(err).Msg("set permissions failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("set permissions: %w", err).Error()})
		}
		return
	}
	c.JSON(http.StatusOK, toUserResponse(u))
}

// UpdateUserPassword sets a new password for a user. Requires user.password.update.
func (h *Handler) UpdateUserPassword(c *gin.Context) {
	userID := c.Param("id")
	var body struct {
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("update password: parse request: %w", err).Error()})
		return
	}
	caller := callerFromContext(c)
	target, err := h.svc.FindUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("update password: %w", err).Error()})
		}
		return
	}
	if !domain.CanManageUser(caller, target) {
		c.JSON(http.StatusForbidden, gin.H{"error": autherrors.ErrManagementForbidden.Error()})
		return
	}
	if err := h.svc.UpdateUserPassword(c.Request.Context(), userID, body.Password); err != nil {
		if errors.Is(err, autherrors.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else if errors.Is(err, autherrors.ErrWeakPassword) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Errorf("update password: %w", err).Error()})
		} else {
			h.logger.Error().Str("event", "update_password_error").Str("user_id", userID).Err(err).Msg("update password failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("update password: %w", err).Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}

// SetUserStatus activates or deactivates a user. Requires user.status.update.
func (h *Handler) SetUserStatus(c *gin.Context) {
	userID := c.Param("id")
	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("set status: parse request: %w", err).Error()})
		return
	}
	caller := callerFromContext(c)
	target, err := h.svc.FindUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("set status: %w", err).Error()})
		}
		return
	}
	if !domain.CanManageUser(caller, target) {
		c.JSON(http.StatusForbidden, gin.H{"error": autherrors.ErrManagementForbidden.Error()})
		return
	}
	u, err := h.svc.SetUserStatus(c.Request.Context(), userID, body.IsActive)
	if err != nil {
		if errors.Is(err, autherrors.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			h.logger.Error().Str("event", "set_status_error").Str("user_id", userID).Err(err).Msg("set status failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Errorf("set status: %w", err).Error()})
		}
		return
	}
	c.JSON(http.StatusOK, toUserResponse(u))
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
		if errors.Is(err, autherrors.ErrInvalidToken) || errors.Is(err, autherrors.ErrTokenExpired) || errors.Is(err, autherrors.ErrAccountDeactivated) {
			h.logger.Warn().
				Str("event", "refresh_rejected").
				Str("ip", ip).
				Err(err).
				Msg("refresh failed: invalid, expired, or deactivated")
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

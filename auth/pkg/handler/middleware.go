package handler

import (
	"errors"
	"net/http"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/gin-gonic/gin"
)

const callerKey = "caller"

// RequireAuth returns a middleware that validates the Bearer access token and
// sets the authenticated user on the Gin context under callerKey.
func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		u, err := h.svc.GetMe(c.Request.Context(), authHeader[7:])
		if err != nil {
			abortGetMeError(c, err)
			return
		}
		c.Set(callerKey, u)
		c.Next()
	}
}

// OptionalAuth loads the caller when a Bearer token is present. Missing auth
// continues anonymously (used for temporary open register). Invalid tokens still 401.
func (h *Handler) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		u, err := h.svc.GetMe(c.Request.Context(), authHeader[7:])
		if err != nil {
			abortGetMeError(c, err)
			return
		}
		c.Set(callerKey, u)
		c.Next()
	}
}

// abortGetMeError writes the appropriate error response for a GetMe failure.
// Shared by RequireAuth and OptionalAuth so both middlewares treat an
// already-issued token for a since-deactivated/locked account the same way
// Login and Refresh do.
func abortGetMeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, autherrors.ErrInvalidToken), errors.Is(err, autherrors.ErrTokenExpired):
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, autherrors.ErrUserNotFound):
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
	case errors.Is(err, autherrors.ErrAccountDeactivated), errors.Is(err, autherrors.ErrAccountLocked):
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
	default:
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth error"})
	}
}

// RequirePermission returns a middleware that rejects callers without the given permission.
// Must be chained after RequireAuth.
func RequirePermission(permission string) gin.HandlerFunc {
	return RequireAnyPermission(permission)
}

// RequireAnyPermission returns a middleware that rejects callers who have none of the given permissions.
// Must be chained after RequireAuth.
func RequireAnyPermission(required ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := callerFromContext(c)
		if u == nil || !u.HasPermission(required...) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permission"})
			return
		}
		c.Next()
	}
}

func callerFromContext(c *gin.Context) *domain.User {
	v, _ := c.Get(callerKey)
	u, _ := v.(*domain.User)
	return u
}

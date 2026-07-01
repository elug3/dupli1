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
			if errors.Is(err, autherrors.ErrInvalidToken) || errors.Is(err, autherrors.ErrTokenExpired) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			} else if errors.Is(err, autherrors.ErrUserNotFound) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			} else {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth error"})
			}
			return
		}
		c.Set(callerKey, u)
		c.Next()
	}
}

// RequireRole returns a middleware that rejects callers without the given role.
// Must be chained after RequireAuth.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := callerFromContext(c)
		if u == nil || !u.HasRole(role) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": role + " role required"})
			return
		}
		c.Next()
	}
}

// RequireAnyRole returns a middleware that rejects callers who have none of the given roles.
// Must be chained after RequireAuth.
func RequireAnyRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := callerFromContext(c)
		if u != nil {
			for _, r := range roles {
				if u.HasRole(r) {
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
	}
}

func callerFromContext(c *gin.Context) *domain.User {
	v, _ := c.Get(callerKey)
	u, _ := v.(*domain.User)
	return u
}

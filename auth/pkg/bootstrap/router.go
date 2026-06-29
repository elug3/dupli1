package bootstrap

import (
	"net/http"

	"github.com/elug3/schick/auth/pkg/domain"
	"github.com/elug3/schick/auth/pkg/handler"
	redisinfra "github.com/elug3/schick/auth/pkg/infra/redis"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func newRouter(h *handler.Handler, debug bool, jwksJSON []byte, redisClient *redis.Client, corsOrigins []string) *gin.Engine {
	if debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware(corsOrigins))

	healthHandler := func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	}
	r.GET("/health", healthHandler)
	r.GET("/api/v1/auth/health", healthHandler)

	if len(jwksJSON) > 0 {
		jwksHandler := func(c *gin.Context) {
			c.Data(http.StatusOK, "application/json", jwksJSON)
		}
		r.GET("/.well-known/jwks.json", jwksHandler)
		r.GET("/api/v1/auth/.well-known/jwks.json", jwksHandler)
	}

	loginLimiter := redisinfra.NewIPRateLimiter(redisClient, "login", 10, 60)
	refreshLimiter := redisinfra.NewIPRateLimiter(redisClient, "refresh", 30, 60)

	v1 := r.Group("/api/v1/auth")
	{
		// Public — no authentication required.
		v1.POST("/register", h.RequireAuth(), handler.RequireAnyRole(
			domain.RoleAdmin,
			domain.RoleUserManager,
			domain.RoleCustomerRegistrar,
		), h.Register)
		v1.POST("/login", loginLimiter.Middleware(), h.Login)
		v1.POST("/refresh", refreshLimiter.Middleware(), h.Refresh)
		v1.POST("/logout", h.Logout) // authenticates via refresh_token in request body

		// Authenticated — require a valid Bearer access token.
		authed := v1.Group("", h.RequireAuth())
		{
			authed.GET("/me", h.Me)
		}

		// Admin-only — require a valid Bearer access token with the "admin" role.
		admin := v1.Group("", h.RequireAuth(), handler.RequireRole(domain.RoleAdmin))
		{
			admin.GET("/users", h.ListUsers)
			admin.PATCH("/users/:id/roles", h.SetUserRole)
		}

		// User management — require "admin" or "user_manager" role.
		userMgmt := v1.Group("", h.RequireAuth(), handler.RequireAnyRole(domain.RoleAdmin, domain.RoleUserManager))
		{
			userMgmt.PATCH("/users/:id/password", h.UpdateUserPassword)
			userMgmt.PATCH("/users/:id/status", h.SetUserStatus)
		}
	}

	return r
}

// corsMiddleware enforces CORS headers for the given list of allowed origins.
// If origins is empty, no CORS headers are added.
func corsMiddleware(origins []string) gin.HandlerFunc {
	if len(origins) == 0 {
		return func(c *gin.Context) { c.Next() }
	}

	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

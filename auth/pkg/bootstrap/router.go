package bootstrap

import (
	"net/http"

	"github.com/elug3/dupli1/auth/pkg/handler"
	redisinfra "github.com/elug3/dupli1/auth/pkg/infra/redis"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/settings"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// NewRouter wires the production auth HTTP routes.
func NewRouter(h *handler.Handler, debug bool, jwksJSON []byte, redisClient *redis.Client, corsOrigins []string) *gin.Engine {
	return newRouter(h, debug, jwksJSON, redisClient, corsOrigins, settings.NewResponse("auth"))
}

func newRouter(h *handler.Handler, debug bool, jwksJSON []byte, redisClient *redis.Client, corsOrigins []string, settingsResp settings.Response) *gin.Engine {
	if debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware(corsOrigins))

	healthHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
	r.GET("/health", healthHandler)
	r.GET("/api/v1/auth/health", healthHandler)

	settingsHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, settingsResp)
	}
	r.GET("/settings", settingsHandler)
	r.GET("/api/v1/auth/settings", settingsHandler)

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
		v1.POST("/register", h.RequireAuth(), handler.RequirePermission(permissions.UserCreate), h.Register)
		v1.POST("/login", loginLimiter.Middleware(), h.Login)
		v1.POST("/refresh", refreshLimiter.Middleware(), h.Refresh)
		v1.POST("/logout", h.Logout)

		authed := v1.Group("", h.RequireAuth())
		{
			authed.GET("/me", h.Me)
		}

		userRead := v1.Group("", h.RequireAuth(), handler.RequirePermission(permissions.UserRead))
		{
			userRead.GET("/users", h.ListUsers)
		}

		userPermissions := v1.Group("", h.RequireAuth(), handler.RequirePermission(permissions.UserPermissionsUpdate))
		{
			userPermissions.PATCH("/users/:id/permissions", h.SetUserPermissions)
		}

		userMgmtPassword := v1.Group("", h.RequireAuth(), handler.RequirePermission(permissions.UserPasswordUpdate))
		{
			userMgmtPassword.PATCH("/users/:id/password", h.UpdateUserPassword)
		}

		userMgmtStatus := v1.Group("", h.RequireAuth(), handler.RequirePermission(permissions.UserStatusUpdate))
		{
			userMgmtStatus.PATCH("/users/:id/status", h.SetUserStatus)
		}
	}

	return r
}

// corsMiddleware enforces CORS headers for the given list of allowed origins.
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

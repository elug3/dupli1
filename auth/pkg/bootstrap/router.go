package bootstrap

import (
	"net/http"

	"github.com/elug3/schick/auth/pkg/handler"
	"github.com/gin-gonic/gin"
)

func newRouter(h *handler.Handler, debug bool) *gin.Engine {
	if debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	healthHandler := func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	}
	r.GET("/health", healthHandler)
	r.GET("/api/v1/auth/health", healthHandler)

	v1 := r.Group("/api/v1/auth")
	{
		// Public — no authentication required.
		v1.POST("/register", h.Register)
		v1.POST("/login", h.Login)
		v1.POST("/refresh", h.Refresh)
		v1.POST("/logout", h.Logout) // authenticates via refresh_token in request body

		// Authenticated — require a valid Bearer access token.
		authed := v1.Group("", h.RequireAuth())
		{
			authed.GET("/me", h.Me)
		}

		// Admin-only — require a valid Bearer access token with the "admin" role.
		admin := v1.Group("", h.RequireAuth(), handler.RequireRole("admin"))
		{
			admin.GET("/users", h.ListUsers)
		}
	}

	return r
}

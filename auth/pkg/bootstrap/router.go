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
		v1.POST("/register", h.Register)
		v1.POST("/login", h.Login)
		v1.POST("/logout", h.Logout)
		v1.POST("/refresh", h.Refresh)
		v1.GET("/me", h.Me)
		v1.GET("/users", h.ListUsers)
	}

	return r
}

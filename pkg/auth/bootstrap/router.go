package bootstrap

import (
	"net/http"

	"github.com/elug3/schick/pkg/auth/handler"
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

	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	v1 := r.Group("/api/v1/auth")
	{
		v1.POST("/register", h.Register)
		v1.POST("/login", h.Login)
		v1.POST("/logout", h.Logout)
		v1.GET("/me", h.Me)
		v1.POST("/refresh", h.Refresh)
	}

	admin := r.Group("/api/v1/users")
	admin.Use(h.RequireAdmin())
	{
		admin.GET("", h.ListUsers)
		admin.POST("", h.CreateUser)
		admin.GET("/:id", h.GetUser)
		admin.PUT("/:id/role", h.UpdateUserRole)
		admin.DELETE("/:id", h.DeleteUser)
	}

	return r
}

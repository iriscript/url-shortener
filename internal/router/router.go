package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/iriscript/url-shortener/internal/router/middleware"
)

type URLHandler interface {
	Shorten(c *gin.Context)
	Redirect(c *gin.Context)
}

func NewRouter(h URLHandler) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.LogMiddleware())
	router.POST("/", h.Shorten)
	router.GET("/:id", h.Redirect)
	return router
}

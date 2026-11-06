package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/router/middleware"
)

type URLHandler interface {
	Shorten(c *gin.Context)
	ShortenJSON(c *gin.Context)
	ShortenBatch(c *gin.Context)
	Redirect(c *gin.Context)
	UserURLs(c *gin.Context)
}

type PingHandler interface {
	Ping(c *gin.Context)
}

func NewRouter(h URLHandler, p PingHandler, auth config.AuthConfig) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.LogMiddleware())
	router.Use(middleware.GzipMiddleware())
	router.Use(middleware.AuthMiddleware(auth.SecretKey))
	router.POST("/", h.Shorten)
	router.POST("/api/shorten", h.ShortenJSON)
	router.POST("/api/shorten/batch", h.ShortenBatch)
	router.GET("/api/user/urls", h.UserURLs)
	router.GET("/ping", p.Ping)
	router.GET("/:id", h.Redirect)
	return router
}

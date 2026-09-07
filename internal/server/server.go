package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/iriscript/url-shortener/internal/config"
)

type URLHandler interface {
	Shorten(c *gin.Context)
	Redirect(c *gin.Context)
}

func NewRouter(h URLHandler) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()
	router.POST("/", h.Shorten)
	router.GET("/:id", h.Redirect)
	return router
}

type Server struct {
	httpServer *http.Server
}

func New(cfg config.ServerConfig, router http.Handler) *Server {
	return &Server{httpServer: &http.Server{Addr: cfg.Address, Handler: router}}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

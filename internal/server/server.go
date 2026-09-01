package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/iriscript/url-shortener/internal/handler"
)

func NewRouter(h *handler.URLHandler) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()
	router.POST("/", h.Shorten)
	router.GET("/:id", h.Redirect)
	return router
}

type Server struct {
	httpServer *http.Server
}

func New(addr string, router http.Handler) *Server {
	return &Server{httpServer: &http.Server{Addr: addr, Handler: router}}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

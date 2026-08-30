package server

import (
	"net/http"

	"github.com/iriscript/url-shortener/internal/handler"
)

func NewRouter(h *handler.URLHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /{$}", h.Shorten)
	mux.HandleFunc("GET /{id}", h.Redirect)
	return mux
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

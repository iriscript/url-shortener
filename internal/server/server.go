package server

import (
	"context"
	"net/http"

	"github.com/iriscript/url-shortener/internal/config"
)

type Server struct {
	httpServer *http.Server
}

func New(cfg config.ServerConfig, router http.Handler) *Server {
	return &Server{httpServer: &http.Server{Addr: cfg.Address, Handler: router}}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

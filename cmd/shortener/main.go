package main

import (
	"log"

	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/handler"
	"github.com/iriscript/url-shortener/internal/repository"
	"github.com/iriscript/url-shortener/internal/server"
)

func main() {
	cfg := config.New()

	repo := repository.NewMemoryRepository()
	h := handler.NewURLHandler(repo, cfg.Handler)
	router := server.NewRouter(h)
	srv := server.New(cfg.Server, router)

	log.Printf("starting server at %s", cfg.Server.Address)
	log.Fatal(srv.Start())
}

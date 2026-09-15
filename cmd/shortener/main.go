package main

import (
	"github.com/iriscript/url-shortener/internal/app/logger"
	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/handler"
	"github.com/iriscript/url-shortener/internal/repository"
	"github.com/iriscript/url-shortener/internal/router"
	"github.com/iriscript/url-shortener/internal/server"
	"github.com/rs/zerolog/log"
)

func main() {
	logger.InitLogger()
	cfg := config.New()

	repo := repository.NewMemoryRepository()
	h := handler.NewURLHandler(repo, cfg.Handler)
	rout := router.NewRouter(h)
	srv := server.New(cfg.Server, rout)

	log.Info().Str("serverAddress", cfg.Server.Address).Msg("Server started")
	err := srv.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

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

	repo, err := newRepository(cfg.Storage)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to init repository")
	}

	h := handler.NewURLHandler(repo, cfg.Handler)
	rout := router.NewRouter(h)
	srv := server.New(cfg.Server, rout)

	log.Info().Str("serverAddress", cfg.Server.Address).Msg("Server started")
	if err := srv.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

func newRepository(cfg config.StorageConfig) (handler.URLRepository, error) {
	if cfg.FileStoragePath == "" {
		log.Info().Msg("File storage path is empty, using in-memory storage")
		return repository.NewMemoryRepository(), nil
	}

	log.Info().Str("fileStoragePath", cfg.FileStoragePath).Msg("Using file storage")

	return repository.NewFileRepository(cfg.FileStoragePath)
}

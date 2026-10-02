package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iriscript/url-shortener/internal/app/logger"
	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/handler"
	"github.com/iriscript/url-shortener/internal/repository"
	"github.com/iriscript/url-shortener/internal/router"
	"github.com/iriscript/url-shortener/internal/server"
	"github.com/rs/zerolog/log"
)

const shutdownTimeout = 5 * time.Second

func main() {
	logger.InitLogger()

	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Server stopped with error")
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.New()

	repo, err := newRepository(cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to init repository: %w", err)
	}
	if closer, ok := repo.(io.Closer); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close repository")
			}
		}()
	}

	h := handler.NewURLHandler(repo, cfg.Handler)
	rout := router.NewRouter(h)
	srv := server.New(cfg.Server, rout)

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)

		log.Info().Str("serverAddress", cfg.Server.Address).Msg("Server started")
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info().Msg("Shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

func newRepository(cfg config.StorageConfig) (handler.URLRepository, error) {
	if cfg.FileStoragePath == "" {
		log.Info().Msg("File storage path is empty, using in-memory storage")
		return repository.NewMemoryRepository(), nil
	}

	log.Info().Str("fileStoragePath", cfg.FileStoragePath).Msg("Using file storage")

	return repository.NewFileRepository(cfg.FileStoragePath)
}

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
	"github.com/iriscript/url-shortener/internal/database"
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

	db, err := database.New(ctx, cfg.Database)
	if err != nil && !errors.Is(err, database.ErrNotConfigured) {
		return fmt.Errorf("failed to init database connection: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close database connection")
		}
	}()

	repo, err := newRepository(db, cfg.Storage)
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
	ping := handler.NewPingHandler(db)
	rout := router.NewRouter(h, ping)
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

func newRepository(db *database.DB, cfg config.StorageConfig) (handler.URLRepository, error) {
	if pool := db.Pool(); pool != nil {
		log.Info().Msg("Using database storage")

		if err := db.Migrate(); err != nil {
			return nil, fmt.Errorf("failed to apply migrations: %w", err)
		}

		return repository.NewPostgresRepository(pool), nil
	}

	if cfg.FileStoragePath != "" {
		log.Info().Str("fileStoragePath", cfg.FileStoragePath).Msg("Using file storage")

		return repository.NewFileRepository(cfg.FileStoragePath)
	}

	log.Info().Msg("Neither database DSN nor file storage path is set, using in-memory storage")

	return repository.NewMemoryRepository(), nil
}

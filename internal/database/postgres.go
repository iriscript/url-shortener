package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iriscript/url-shortener/internal/config"
)

var ErrNotConfigured = errors.New("database is not configured")

type DB struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	if cfg.DSN == "" {
		return nil, ErrNotConfigured
	}

	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return &DB{pool: pool}, nil
}

func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return ErrNotConfigured
	}

	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping the database: %w", err)
	}

	return nil
}

func (db *DB) Close() error {
	if db == nil || db.pool == nil {
		return nil
	}

	db.pool.Close()
	db.pool = nil

	return nil
}

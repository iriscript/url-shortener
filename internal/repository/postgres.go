package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Save(ctx context.Context, id, originalURL string) error {
	const query = `INSERT INTO short_urls (short_id, original_url) VALUES ($1, $2)`

	_, err := r.pool.Exec(ctx, query, id, originalURL)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return ErrIDConflict
		}

		return fmt.Errorf("failed to insert the URL: %w", err)
	}

	return nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (string, bool) {
	const query = `SELECT original_url FROM short_urls WHERE short_id = $1`

	var originalURL string
	err := r.pool.QueryRow(ctx, query, id).Scan(&originalURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false
	}
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("postgres: failed to select the URL")
		return "", false
	}

	return originalURL, true
}

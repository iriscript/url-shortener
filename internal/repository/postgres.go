package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iriscript/url-shortener/internal/database"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{pool: db.Pool()}
}

func (r *PostgresRepository) Save(ctx context.Context, id, originalURL string) (string, error) {
	const query = `INSERT INTO short_urls (short_id, original_url)
		VALUES ($1, $2)
		ON CONFLICT (original_url) DO NOTHING
		RETURNING short_id`

	var storedID string
	err := r.pool.QueryRow(ctx, query, id, originalURL).Scan(&storedID)
	if errors.Is(err, pgx.ErrNoRows) {
		existingID, lookupErr := r.getIDByURL(ctx, originalURL)
		if lookupErr != nil {
			return "", lookupErr
		}

		return existingID, ErrURLConflict
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return "", ErrIDConflict
		}

		return "", fmt.Errorf("failed to insert the URL: %w", err)
	}

	return storedID, nil
}

func (r *PostgresRepository) SaveBatch(ctx context.Context, records []URLRecord) ([]URLRecord, error) {
	const query = `WITH input AS (
			SELECT *
			FROM unnest($1::text[], $2::text[]) WITH ORDINALITY AS t(short_id, original_url, ord)
		), inserted AS (
			INSERT INTO short_urls (short_id, original_url)
			SELECT short_id, original_url FROM input
			ON CONFLICT (original_url) DO NOTHING
			RETURNING short_id, original_url
		)
		SELECT input.original_url, COALESCE(inserted.short_id, existing.short_id)
		FROM input
		LEFT JOIN inserted ON inserted.original_url = input.original_url
		LEFT JOIN short_urls existing ON existing.original_url = input.original_url
		ORDER BY input.ord`

	ids := make([]string, len(records))
	originalURLs := make([]string, len(records))
	for i, rec := range records {
		ids[i] = rec.ID
		originalURLs[i] = rec.OriginalURL
	}

	rows, err := r.pool.Query(ctx, query, ids, originalURLs)
	if err != nil {
		return nil, wrapBatchError(err)
	}
	defer rows.Close()

	stored := make([]URLRecord, 0, len(records))
	for rows.Next() {
		var rec URLRecord
		if err := rows.Scan(&rec.OriginalURL, &rec.ID); err != nil {
			return nil, fmt.Errorf("failed to read the stored URL batch: %w", err)
		}

		stored = append(stored, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapBatchError(err)
	}

	if len(stored) != len(records) {
		return nil, fmt.Errorf("stored %d of %d records in the batch", len(stored), len(records))
	}

	return stored, nil
}

func wrapBatchError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return ErrIDConflict
	}

	return fmt.Errorf("failed to insert the URL batch: %w", err)
}

func (r *PostgresRepository) getIDByURL(ctx context.Context, originalURL string) (string, error) {
	const query = `SELECT short_id FROM short_urls WHERE original_url = $1`

	var id string
	if err := r.pool.QueryRow(ctx, query, originalURL).Scan(&id); err != nil {
		return "", fmt.Errorf("failed to select the existing URL: %w", err)
	}

	return id, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (string, error) {
	const query = `SELECT original_url FROM short_urls WHERE short_id = $1`

	var originalURL string
	err := r.pool.QueryRow(ctx, query, id).Scan(&originalURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("postgres: failed to select the url: %w", err)
	}

	return originalURL, nil
}

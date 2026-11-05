package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/database"
	"github.com/iriscript/url-shortener/internal/repository"
)

// newTestPool connects to the database named by TEST_DATABASE_DSN, applies the
// migrations and hands back a pool. Without that variable the test is skipped,
// so `go test ./...` stays runnable without a database.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set, skipping the PostgreSQL integration test")
	}

	ctx := t.Context()

	db, err := database.New(ctx, config.DatabaseConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("failed to reach the database: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db.Pool()
}

func TestPostgresRepository_SaveAndGet(t *testing.T) {
	const (
		id          = "pgTest01"
		originalURL = "https://practicum.yandex.ru/"
	)

	ctx := t.Context()
	pool := newTestPool(t)
	repo := repository.NewPostgresRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM short_urls WHERE short_id = $1`, id)
	})

	if err := repo.Save(ctx, id, originalURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, ok := repo.Get(ctx, id)
	if !ok {
		t.Fatal("Get did not find the saved URL")
	}
	if got != originalURL {
		t.Errorf("Get = %q, want %q", got, originalURL)
	}
}

func TestPostgresRepository_SaveReturnsIDConflict(t *testing.T) {
	const (
		id          = "pgTest02"
		originalURL = "https://practicum.yandex.ru/"
	)

	ctx := t.Context()
	pool := newTestPool(t)
	repo := repository.NewPostgresRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM short_urls WHERE short_id = $1`, id)
	})

	if err := repo.Save(ctx, id, originalURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	err := repo.Save(ctx, id, "https://ya.ru/")
	if !errors.Is(err, repository.ErrIDConflict) {
		t.Errorf("Save error = %v, want %v", err, repository.ErrIDConflict)
	}
}

func TestPostgresRepository_SaveBatch(t *testing.T) {
	records := []repository.URLRecord{
		{ID: "pgBatch1", OriginalURL: "https://practicum.yandex.ru/"},
		{ID: "pgBatch2", OriginalURL: "https://ya.ru/"},
	}

	ctx := t.Context()
	pool := newTestPool(t)
	repo := repository.NewPostgresRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM short_urls WHERE short_id LIKE 'pgBatch%'`)
	})

	if err := repo.SaveBatch(ctx, records); err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	for _, rec := range records {
		got, ok := repo.Get(ctx, rec.ID)
		if !ok {
			t.Fatalf("Get(%q) found nothing", rec.ID)
		}
		if got != rec.OriginalURL {
			t.Errorf("Get(%q) = %q, want %q", rec.ID, got, rec.OriginalURL)
		}
	}
}

func TestPostgresRepository_SaveBatchIsAtomicOnConflict(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	repo := repository.NewPostgresRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM short_urls WHERE short_id LIKE 'pgAtomic%'`)
	})

	if err := repo.Save(ctx, "pgAtomic1", "https://practicum.yandex.ru/"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	err := repo.SaveBatch(ctx, []repository.URLRecord{
		{ID: "pgAtomic2", OriginalURL: "https://ya.ru/"},
		{ID: "pgAtomic1", OriginalURL: "https://go.dev/"},
	})
	if !errors.Is(err, repository.ErrIDConflict) {
		t.Fatalf("SaveBatch error = %v, want %v", err, repository.ErrIDConflict)
	}

	if _, ok := repo.Get(ctx, "pgAtomic2"); ok {
		t.Error("a record from the rejected batch was stored")
	}
}

func TestPostgresRepository_GetUnknownID(t *testing.T) {
	ctx := t.Context()
	repo := repository.NewPostgresRepository(newTestPool(t))

	if _, ok := repo.Get(ctx, "noSuchID"); ok {
		t.Error("Get found a URL for an unknown id")
	}
}

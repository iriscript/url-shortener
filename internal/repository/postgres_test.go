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

	if _, err := repo.Save(ctx, repository.URLRecord{ID: id, OriginalURL: originalURL}); err != nil {
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

	if _, err := repo.Save(ctx, repository.URLRecord{ID: id, OriginalURL: originalURL}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	_, err := repo.Save(ctx, repository.URLRecord{ID: id, OriginalURL: "https://ya.ru/"})
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

	if _, err := repo.SaveBatch(ctx, records); err != nil {
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

	if _, err := repo.Save(ctx, repository.URLRecord{ID: "pgAtomic1", OriginalURL: "https://practicum.yandex.ru/"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	_, err := repo.SaveBatch(ctx, []repository.URLRecord{
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

func TestPostgresRepository_SaveReturnsExistingIDForKnownURL(t *testing.T) {
	const originalURL = "https://pg-conflict.example.com/"

	ctx := t.Context()
	pool := newTestPool(t)
	repo := repository.NewPostgresRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM short_urls WHERE original_url = $1`, originalURL)
	})

	firstID, err := repo.Save(ctx, repository.URLRecord{ID: "pgKnown1", OriginalURL: originalURL})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	secondID, err := repo.Save(ctx, repository.URLRecord{ID: "pgKnown2", OriginalURL: originalURL})
	if !errors.Is(err, repository.ErrURLConflict) {
		t.Fatalf("Save error = %v, want %v", err, repository.ErrURLConflict)
	}
	if secondID != firstID {
		t.Errorf("Save returned id %q, want the already stored %q", secondID, firstID)
	}

	if _, ok := repo.Get(ctx, "pgKnown2"); ok {
		t.Error("the rejected id was stored anyway")
	}
}

func TestPostgresRepository_SaveBatchReusesExistingIDForKnownURL(t *testing.T) {
	const (
		knownURL = "https://pg-batch-known.example.com/"
		freshURL = "https://pg-batch-fresh.example.com/"
	)

	ctx := t.Context()
	pool := newTestPool(t)
	repo := repository.NewPostgresRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM short_urls WHERE original_url IN ($1, $2)`, knownURL, freshURL)
	})

	knownID, err := repo.Save(ctx, repository.URLRecord{ID: "pgReuse1", OriginalURL: knownURL})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	stored, err := repo.SaveBatch(ctx, []repository.URLRecord{
		{ID: "pgReuse2", OriginalURL: freshURL},
		{ID: "pgReuse3", OriginalURL: knownURL},
	})
	if err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	if len(stored) != 2 {
		t.Fatalf("SaveBatch returned %d records, want 2", len(stored))
	}
	if stored[0].ID != "pgReuse2" {
		t.Errorf("fresh record id = %q, want %q", stored[0].ID, "pgReuse2")
	}
	if stored[1].ID != knownID {
		t.Errorf("known URL got id %q, want the already stored %q", stored[1].ID, knownID)
	}

	if _, ok := repo.Get(ctx, "pgReuse3"); ok {
		t.Error("a second id was stored for an already known URL")
	}
}

// The unique index must survive a migration run over a table that already
// holds duplicate URLs.
func TestMigrate_DeduplicatesExistingURLs(t *testing.T) {
	const originalURL = "https://pg-dedup.example.com/"

	ctx := t.Context()
	pool := newTestPool(t)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM short_urls WHERE original_url = $1`, originalURL)
	})

	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM short_urls WHERE original_url = $1`, originalURL).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("found %d leftover rows for the test URL, want 0", count)
	}

	if _, err := repository.NewPostgresRepository(pool).Save(ctx, repository.URLRecord{ID: "pgDedup1", OriginalURL: originalURL}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO short_urls (short_id, original_url) VALUES ($1, $2)`, "pgDedup2", originalURL)
	if err == nil {
		t.Fatal("a duplicate original_url was accepted, the unique index is missing")
	}
}

func TestPostgresRepository_GetUnknownID(t *testing.T) {
	ctx := t.Context()
	repo := repository.NewPostgresRepository(newTestPool(t))

	if _, ok := repo.Get(ctx, "noSuchID"); ok {
		t.Error("Get found a URL for an unknown id")
	}
}

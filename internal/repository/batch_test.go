package repository_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/iriscript/url-shortener/internal/repository"
)

type batchRepository interface {
	Save(ctx context.Context, id, originalURL string) error
	SaveBatch(ctx context.Context, records []repository.URLRecord) error
	Get(ctx context.Context, id string) (string, bool)
}

func batchBackends(t *testing.T) map[string]batchRepository {
	t.Helper()

	fileRepo, err := repository.NewFileRepository(filepath.Join(t.TempDir(), "batch.json"))
	if err != nil {
		t.Fatalf("failed to create the file repository: %v", err)
	}
	t.Cleanup(func() { _ = fileRepo.Close() })

	return map[string]batchRepository{
		"memory": repository.NewMemoryRepository(),
		"file":   fileRepo,
	}
}

func TestSaveBatch_StoresEveryRecord(t *testing.T) {
	records := []repository.URLRecord{
		{ID: "batchId1", OriginalURL: "https://practicum.yandex.ru/"},
		{ID: "batchId2", OriginalURL: "https://ya.ru/"},
	}

	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

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
		})
	}
}

func TestSaveBatch_RejectsIDAlreadyStoredAndWritesNothing(t *testing.T) {
	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			if err := repo.Save(ctx, "existing", "https://practicum.yandex.ru/"); err != nil {
				t.Fatalf("Save failed: %v", err)
			}

			err := repo.SaveBatch(ctx, []repository.URLRecord{
				{ID: "fresh001", OriginalURL: "https://ya.ru/"},
				{ID: "existing", OriginalURL: "https://go.dev/"},
			})
			if !errors.Is(err, repository.ErrIDConflict) {
				t.Fatalf("SaveBatch error = %v, want %v", err, repository.ErrIDConflict)
			}

			if _, ok := repo.Get(ctx, "fresh001"); ok {
				t.Error("a record from the rejected batch was stored")
			}
		})
	}
}

func TestSaveBatch_RejectsDuplicateIDsInsideTheBatch(t *testing.T) {
	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			err := repo.SaveBatch(ctx, []repository.URLRecord{
				{ID: "sameIdAA", OriginalURL: "https://practicum.yandex.ru/"},
				{ID: "sameIdAA", OriginalURL: "https://ya.ru/"},
			})
			if !errors.Is(err, repository.ErrIDConflict) {
				t.Fatalf("SaveBatch error = %v, want %v", err, repository.ErrIDConflict)
			}

			if _, ok := repo.Get(ctx, "sameIdAA"); ok {
				t.Error("a record from the rejected batch was stored")
			}
		})
	}
}

// TestSaveBatch_IsSafeUnderConcurrency is meaningful under -race: concurrent
// batches must not corrupt the index or lose records.
func TestSaveBatch_IsSafeUnderConcurrency(t *testing.T) {
	const (
		goroutines     = 8
		recordsPerCall = 5
	)

	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			var wg sync.WaitGroup
			for g := range goroutines {
				wg.Add(1)
				go func(g int) {
					defer wg.Done()

					records := make([]repository.URLRecord, recordsPerCall)
					for i := range records {
						records[i] = repository.URLRecord{
							ID:          fmt.Sprintf("g%02di%02d", g, i),
							OriginalURL: fmt.Sprintf("https://example.com/%d/%d", g, i),
						}
					}

					if err := repo.SaveBatch(ctx, records); err != nil {
						t.Errorf("SaveBatch failed: %v", err)
					}
				}(g)
			}
			wg.Wait()

			for g := range goroutines {
				for i := range recordsPerCall {
					id := fmt.Sprintf("g%02di%02d", g, i)
					want := fmt.Sprintf("https://example.com/%d/%d", g, i)

					got, ok := repo.Get(ctx, id)
					if !ok {
						t.Errorf("Get(%q) found nothing", id)
						continue
					}
					if got != want {
						t.Errorf("Get(%q) = %q, want %q", id, got, want)
					}
				}
			}
		})
	}
}

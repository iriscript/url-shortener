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
	Save(ctx context.Context, record repository.URLRecord) (string, error)
	SaveBatch(ctx context.Context, records []repository.URLRecord) ([]repository.URLRecord, error)
	Get(ctx context.Context, id string) (string, bool)
	GetByUser(ctx context.Context, userID string) ([]repository.URLRecord, error)
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
		})
	}
}

func TestSaveBatch_RejectsIDAlreadyStoredAndWritesNothing(t *testing.T) {
	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			if _, err := repo.Save(ctx, repository.URLRecord{ID: "existing", OriginalURL: "https://practicum.yandex.ru/"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}

			_, err := repo.SaveBatch(ctx, []repository.URLRecord{
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

			_, err := repo.SaveBatch(ctx, []repository.URLRecord{
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

					if _, err := repo.SaveBatch(ctx, records); err != nil {
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

func TestSave_ReturnsExistingIDForKnownURL(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			firstID, err := repo.Save(ctx, repository.URLRecord{ID: "firstId1", OriginalURL: originalURL})
			if err != nil {
				t.Fatalf("Save failed: %v", err)
			}
			if firstID != "firstId1" {
				t.Fatalf("Save returned id %q, want %q", firstID, "firstId1")
			}

			secondID, err := repo.Save(ctx, repository.URLRecord{ID: "secondId", OriginalURL: originalURL})
			if !errors.Is(err, repository.ErrURLConflict) {
				t.Fatalf("Save error = %v, want %v", err, repository.ErrURLConflict)
			}
			if secondID != firstID {
				t.Errorf("Save returned id %q, want the already stored %q", secondID, firstID)
			}

			if got, _ := repo.Get(ctx, "secondId"); got != "" {
				t.Error("the rejected id was stored anyway")
			}
		})
	}
}

func TestGetByUser_ReturnsOnlyTheRecordsOfThatUser(t *testing.T) {
	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			if _, err := repo.Save(ctx, repository.URLRecord{ID: "aliceId1", OriginalURL: "https://practicum.yandex.ru/", UserID: "alice"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}
			if _, err := repo.Save(ctx, repository.URLRecord{ID: "bobId001", OriginalURL: "https://ya.ru/", UserID: "bob"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}
			if _, err := repo.SaveBatch(ctx, []repository.URLRecord{
				{ID: "aliceId2", OriginalURL: "https://go.dev/", UserID: "alice"},
			}); err != nil {
				t.Fatalf("SaveBatch failed: %v", err)
			}

			records, err := repo.GetByUser(ctx, "alice")
			if err != nil {
				t.Fatalf("GetByUser failed: %v", err)
			}

			want := []repository.URLRecord{
				{ID: "aliceId1", OriginalURL: "https://practicum.yandex.ru/", UserID: "alice"},
				{ID: "aliceId2", OriginalURL: "https://go.dev/", UserID: "alice"},
			}
			if len(records) != len(want) {
				t.Fatalf("GetByUser returned %d records, want %d: %+v", len(records), len(want), records)
			}
			for i := range want {
				if records[i] != want[i] {
					t.Errorf("record %d = %+v, want %+v", i, records[i], want[i])
				}
			}
		})
	}
}

func TestGetByUser_ReturnsNothingForAnUnknownUser(t *testing.T) {
	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			records, err := repo.GetByUser(t.Context(), "nobody")
			if err != nil {
				t.Fatalf("GetByUser failed: %v", err)
			}
			if len(records) != 0 {
				t.Errorf("GetByUser = %+v, want no records", records)
			}
		})
	}
}

func TestSaveBatch_ReusesExistingIDForKnownURL(t *testing.T) {
	const knownURL = "https://practicum.yandex.ru/"

	for name, repo := range batchBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			knownID, err := repo.Save(ctx, repository.URLRecord{ID: "knownId1", OriginalURL: knownURL})
			if err != nil {
				t.Fatalf("Save failed: %v", err)
			}

			stored, err := repo.SaveBatch(ctx, []repository.URLRecord{
				{ID: "newId001", OriginalURL: "https://ya.ru/"},
				{ID: "newId002", OriginalURL: knownURL},
			})
			if err != nil {
				t.Fatalf("SaveBatch failed: %v", err)
			}

			if len(stored) != 2 {
				t.Fatalf("SaveBatch returned %d records, want 2", len(stored))
			}
			if stored[0].ID != "newId001" {
				t.Errorf("fresh record id = %q, want %q", stored[0].ID, "newId001")
			}
			if stored[1].ID != knownID {
				t.Errorf("known URL got id %q, want the already stored %q", stored[1].ID, knownID)
			}

			if got, _ := repo.Get(ctx, "newId002"); got != "" {
				t.Error("a second id was stored for an already known URL")
			}
		})
	}
}

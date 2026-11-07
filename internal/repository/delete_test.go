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

type deleteRepository interface {
	Save(ctx context.Context, rec repository.URLRecord) (string, error)
	Get(ctx context.Context, id string) (string, error)
	GetByUser(ctx context.Context, userID string) ([]repository.URLRecord, error)
	MarkDeleted(ctx context.Context, records []repository.DeleteRecord) error
}

func deleteBackends(t *testing.T) map[string]deleteRepository {
	t.Helper()

	fileRepo, err := repository.NewFileRepository(filepath.Join(t.TempDir(), "delete.json"))
	if err != nil {
		t.Fatalf("failed to create the file repository: %v", err)
	}
	t.Cleanup(func() { _ = fileRepo.Close() })

	return map[string]deleteRepository{
		"memory": repository.NewMemoryRepository(),
		"file":   fileRepo,
	}
}

func TestMarkDeleted_OwnerCanDeleteTheirURL(t *testing.T) {
	for name, repo := range deleteBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			if _, err := repo.Save(ctx, repository.URLRecord{ID: "ownedId1", OriginalURL: "https://practicum.yandex.ru/", UserID: "owner"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}

			if err := repo.MarkDeleted(ctx, []repository.DeleteRecord{{ID: "ownedId1", UserID: "owner"}}); err != nil {
				t.Fatalf("MarkDeleted failed: %v", err)
			}

			if _, err := repo.Get(ctx, "ownedId1"); !errors.Is(err, repository.ErrDeleted) {
				t.Fatalf("Get error = %v, want %v", err, repository.ErrDeleted)
			}
		})
	}
}

func TestMarkDeleted_RespectsOwnership(t *testing.T) {
	for name, repo := range deleteBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			if _, err := repo.Save(ctx, repository.URLRecord{ID: "ownedId2", OriginalURL: "https://ya.ru/", UserID: "owner"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}

			if err := repo.MarkDeleted(ctx, []repository.DeleteRecord{{ID: "ownedId2", UserID: "stranger"}}); err != nil {
				t.Fatalf("MarkDeleted failed: %v", err)
			}

			originalURL, err := repo.Get(ctx, "ownedId2")
			if err != nil {
				t.Fatalf("Get failed: %v, want the url to still be reachable", err)
			}
			if originalURL != "https://ya.ru/" {
				t.Errorf("Get = %q, want %q", originalURL, "https://ya.ru/")
			}
		})
	}
}

func TestMarkDeleted_ExcludesDeletedFromGetByUser(t *testing.T) {
	for name, repo := range deleteBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			if _, err := repo.Save(ctx, repository.URLRecord{ID: "keepId01", OriginalURL: "https://go.dev/", UserID: "owner"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}
			if _, err := repo.Save(ctx, repository.URLRecord{ID: "dropId01", OriginalURL: "https://golang.org/", UserID: "owner"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}

			if err := repo.MarkDeleted(ctx, []repository.DeleteRecord{{ID: "dropId01", UserID: "owner"}}); err != nil {
				t.Fatalf("MarkDeleted failed: %v", err)
			}

			records, err := repo.GetByUser(ctx, "owner")
			if err != nil {
				t.Fatalf("GetByUser failed: %v", err)
			}

			for _, rec := range records {
				if rec.ID == "dropId01" {
					t.Errorf("GetByUser returned the deleted id %q", rec.ID)
				}
			}
			if len(records) != 1 || records[0].ID != "keepId01" {
				t.Errorf("GetByUser = %+v, want only keepId01", records)
			}
		})
	}
}

func TestMarkDeleted_AllowsReshorteningTheSameURL(t *testing.T) {
	for name, repo := range deleteBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			if _, err := repo.Save(ctx, repository.URLRecord{ID: "goneId01", OriginalURL: "https://practicum.yandex.ru/", UserID: "owner"}); err != nil {
				t.Fatalf("Save failed: %v", err)
			}
			if err := repo.MarkDeleted(ctx, []repository.DeleteRecord{{ID: "goneId01", UserID: "owner"}}); err != nil {
				t.Fatalf("MarkDeleted failed: %v", err)
			}

			newID, err := repo.Save(ctx, repository.URLRecord{ID: "freshId1", OriginalURL: "https://practicum.yandex.ru/", UserID: "owner"})
			if err != nil {
				t.Fatalf("Save after delete failed: %v, want the url to be re-shortenable", err)
			}
			if newID != "freshId1" {
				t.Errorf("Save after delete returned id %q, want the fresh id %q", newID, "freshId1")
			}

			originalURL, err := repo.Get(ctx, "freshId1")
			if err != nil {
				t.Fatalf("Get(%q) failed: %v", "freshId1", err)
			}
			if originalURL != "https://practicum.yandex.ru/" {
				t.Errorf("Get(%q) = %q, want %q", "freshId1", originalURL, "https://practicum.yandex.ru/")
			}
		})
	}
}

func TestMarkDeleted_IsSafeUnderConcurrency(t *testing.T) {
	const goroutines = 8

	for name, repo := range deleteBackends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			ids := make([]string, goroutines)
			for g := range goroutines {
				id := fmt.Sprintf("conc%02d", g)
				ids[g] = id
				if _, err := repo.Save(ctx, repository.URLRecord{ID: id, OriginalURL: fmt.Sprintf("https://example.com/%d", g), UserID: "owner"}); err != nil {
					t.Fatalf("Save failed: %v", err)
				}
			}

			var wg sync.WaitGroup
			for _, id := range ids {
				wg.Add(1)
				go func(id string) {
					defer wg.Done()

					if err := repo.MarkDeleted(ctx, []repository.DeleteRecord{{ID: id, UserID: "owner"}}); err != nil {
						t.Errorf("MarkDeleted(%q) failed: %v", id, err)
					}
				}(id)
			}
			wg.Wait()

			for _, id := range ids {
				if _, err := repo.Get(ctx, id); !errors.Is(err, repository.ErrDeleted) {
					t.Errorf("Get(%q) error = %v, want %v", id, err, repository.ErrDeleted)
				}
			}
		})
	}
}

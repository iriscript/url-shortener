package repository

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrIDConflict  = errors.New("id already exists")
	ErrURLConflict = errors.New("original url already exists")
	ErrClosed      = errors.New("repository is closed")
)

type URLRecord struct {
	ID          string
	OriginalURL string
	UserID      string
}

type MemoryRepository struct {
	mu     sync.Mutex
	m      map[string]string
	byURL  map[string]string
	byUser map[string][]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		m:      make(map[string]string),
		byURL:  make(map[string]string),
		byUser: make(map[string][]string),
	}
}

func (r *MemoryRepository) Save(_ context.Context, rec URLRecord) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existingID, exists := r.byURL[rec.OriginalURL]; exists {
		return existingID, ErrURLConflict
	}
	if _, exists := r.m[rec.ID]; exists {
		return "", ErrIDConflict
	}

	r.store(rec)

	return rec.ID, nil
}

func (r *MemoryRepository) SaveBatch(_ context.Context, records []URLRecord) ([]URLRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored := make([]URLRecord, len(records))
	seen := make(map[string]struct{}, len(records))

	for i, rec := range records {
		if existingID, exists := r.byURL[rec.OriginalURL]; exists {
			stored[i] = URLRecord{ID: existingID, OriginalURL: rec.OriginalURL, UserID: rec.UserID}
			continue
		}
		if _, exists := r.m[rec.ID]; exists {
			return nil, ErrIDConflict
		}
		if _, duplicate := seen[rec.ID]; duplicate {
			return nil, ErrIDConflict
		}

		seen[rec.ID] = struct{}{}
		stored[i] = rec
	}

	for i, rec := range stored {
		if _, alreadyStored := r.m[rec.ID]; alreadyStored {
			continue
		}

		r.store(URLRecord{ID: rec.ID, OriginalURL: records[i].OriginalURL, UserID: rec.UserID})
	}

	return stored, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	originalURL, ok := r.m[id]
	return originalURL, ok
}

func (r *MemoryRepository) GetByUser(_ context.Context, userID string) ([]URLRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := r.byUser[userID]
	records := make([]URLRecord, 0, len(ids))
	for _, id := range ids {
		records = append(records, URLRecord{ID: id, OriginalURL: r.m[id], UserID: userID})
	}

	return records, nil
}

// store indexes a record that is known to be absent. The caller must hold the mutex.
func (r *MemoryRepository) store(rec URLRecord) {
	r.m[rec.ID] = rec.OriginalURL
	r.byURL[rec.OriginalURL] = rec.ID
	if rec.UserID != "" {
		r.byUser[rec.UserID] = append(r.byUser[rec.UserID], rec.ID)
	}
}

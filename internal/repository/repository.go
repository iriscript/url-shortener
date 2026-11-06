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
	ErrNotFound    = errors.New("short url not found")
)

type URLRecord struct {
	ID          string
	OriginalURL string
}

type MemoryRepository struct {
	mu    sync.Mutex
	m     map[string]string
	byURL map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		m:     make(map[string]string),
		byURL: make(map[string]string),
	}
}

func (r *MemoryRepository) Save(_ context.Context, id, originalURL string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existingID, exists := r.byURL[originalURL]; exists {
		return existingID, ErrURLConflict
	}
	if _, exists := r.m[id]; exists {
		return "", ErrIDConflict
	}

	r.m[id] = originalURL
	r.byURL[originalURL] = id

	return id, nil
}

func (r *MemoryRepository) SaveBatch(_ context.Context, records []URLRecord) ([]URLRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored := make([]URLRecord, len(records))
	seen := make(map[string]struct{}, len(records))

	for i, rec := range records {
		if existingID, exists := r.byURL[rec.OriginalURL]; exists {
			stored[i] = URLRecord{ID: existingID, OriginalURL: rec.OriginalURL}
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

	for _, rec := range stored {
		if _, alreadyStored := r.m[rec.ID]; alreadyStored {
			continue
		}

		r.m[rec.ID] = rec.OriginalURL
		r.byURL[rec.OriginalURL] = rec.ID
	}

	return stored, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	originalURL, ok := r.m[id]
	if !ok {
		return "", ErrNotFound
	}

	return originalURL, nil
}

package repository

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrIDConflict = errors.New("id already exists")
	ErrClosed     = errors.New("repository is closed")
)

type URLRecord struct {
	ID          string
	OriginalURL string
}

type MemoryRepository struct {
	mu sync.Mutex
	m  map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{m: make(map[string]string)}
}

func (r *MemoryRepository) Save(_ context.Context, id, originalURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.m[id]; exists {
		return ErrIDConflict
	}
	r.m[id] = originalURL

	return nil
}

func (r *MemoryRepository) SaveBatch(_ context.Context, records []URLRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[string]struct{}, len(records))
	for _, rec := range records {
		if _, exists := r.m[rec.ID]; exists {
			return ErrIDConflict
		}
		if _, duplicate := seen[rec.ID]; duplicate {
			return ErrIDConflict
		}
		seen[rec.ID] = struct{}{}
	}

	for _, rec := range records {
		r.m[rec.ID] = rec.OriginalURL
	}

	return nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	originalURL, ok := r.m[id]
	return originalURL, ok
}

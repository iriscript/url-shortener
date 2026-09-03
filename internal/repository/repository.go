package repository

import (
	"errors"
	"sync"
)

var ErrIDConflict = errors.New("id already exists")

type MemoryRepository struct {
	mu sync.Mutex
	m  map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{m: make(map[string]string)}
}

func (r *MemoryRepository) Save(id, originalURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.m[id]; exists {
		return ErrIDConflict
	}
	r.m[id] = originalURL

	return nil
}

func (r *MemoryRepository) Get(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	originalURL, ok := r.m[id]
	return originalURL, ok
}

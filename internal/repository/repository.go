package repository

import (
	"math/rand"
	"sync"
)

const (
	idAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	idLength   = 8
)

type URLRepository interface {
	Save(originalURL string) (id string)
	Get(id string) (originalURL string, ok bool)
}

type MemoryRepository struct {
	mu sync.RWMutex
	m  map[string]string
}

var _ URLRepository = (*MemoryRepository)(nil)

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{m: make(map[string]string)}
}

func (r *MemoryRepository) Save(originalURL string) string {
	id := generateID()

	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		if _, exists := r.m[id]; !exists {
			break
		}
		id = generateID()
	}
	r.m[id] = originalURL

	return id
}

func (r *MemoryRepository) Get(id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	originalURL, ok := r.m[id]
	return originalURL, ok
}

func generateID() string {
	b := make([]byte, idLength)
	for i := range b {
		b[i] = idAlphabet[rand.Intn(len(idAlphabet))]
	}
	return string(b)
}

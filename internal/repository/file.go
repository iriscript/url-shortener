package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const (
	storageDirPerm  = 0o755
	storageFilePerm = 0o600
)

type record struct {
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

type FileRepository struct {
	mu      sync.Mutex
	index   map[string]string
	byURL   map[string]string
	count   int
	path    string
	file    *os.File
	encoder *json.Encoder
}

func NewFileRepository(path string) (*FileRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), storageDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	r := &FileRepository{
		index: make(map[string]string),
		byURL: make(map[string]string),
		path:  path,
	}

	if err := r.load(); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, storageFilePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open storage file: %w", err)
	}

	r.file = file
	r.encoder = json.NewEncoder(file)

	return r, nil
}

func (r *FileRepository) load() error {
	file, err := os.Open(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to open storage file: %w", err)
	}
	defer func() { _ = file.Close() }()

	decoder := json.NewDecoder(file)
	for {
		var rec record
		if err := decoder.Decode(&rec); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("failed to parse storage file: %w", err)
		}

		r.index[rec.ShortURL] = rec.OriginalURL
		r.byURL[rec.OriginalURL] = rec.ShortURL
		r.count++
	}
}

func (r *FileRepository) Save(_ context.Context, id, originalURL string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existingID, exists := r.byURL[originalURL]; exists {
		return existingID, ErrURLConflict
	}
	if _, exists := r.index[id]; exists {
		return "", ErrIDConflict
	}

	if err := r.store(id, originalURL); err != nil {
		return "", err
	}

	return id, nil
}

func (r *FileRepository) SaveBatch(_ context.Context, records []URLRecord) ([]URLRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored := make([]URLRecord, len(records))
	fresh := make([]int, 0, len(records))
	seen := make(map[string]struct{}, len(records))

	for i, rec := range records {
		if existingID, exists := r.byURL[rec.OriginalURL]; exists {
			stored[i] = URLRecord{ID: existingID, OriginalURL: rec.OriginalURL}
			continue
		}
		if _, exists := r.index[rec.ID]; exists {
			return nil, ErrIDConflict
		}
		if _, duplicate := seen[rec.ID]; duplicate {
			return nil, ErrIDConflict
		}

		seen[rec.ID] = struct{}{}
		stored[i] = rec
		fresh = append(fresh, i)
	}

	for _, i := range fresh {
		if err := r.store(stored[i].ID, stored[i].OriginalURL); err != nil {
			return nil, err
		}
	}

	return stored, nil
}

func (r *FileRepository) store(id, originalURL string) error {
	rec := record{
		UUID:        strconv.Itoa(r.count + 1),
		ShortURL:    id,
		OriginalURL: originalURL,
	}

	if err := r.append(rec); err != nil {
		return err
	}

	r.index[id] = originalURL
	r.byURL[originalURL] = id
	r.count++

	return nil
}

func (r *FileRepository) Get(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	originalURL, ok := r.index[id]
	if !ok {
		return "", ErrNotFound
	}

	return originalURL, nil
}

func (r *FileRepository) append(rec record) error {
	if r.file == nil {
		return ErrClosed
	}

	if err := r.encoder.Encode(rec); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	return nil
}

func (r *FileRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file == nil {
		return nil
	}

	err := r.file.Close()
	r.file = nil
	r.encoder = nil

	if err != nil {
		return fmt.Errorf("failed to close storage file: %w", err)
	}

	return nil
}

package repository

import (
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
		r.count++
	}
}

func (r *FileRepository) Save(id, originalURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.index[id]; exists {
		return ErrIDConflict
	}

	rec := record{
		UUID:        strconv.Itoa(r.count + 1),
		ShortURL:    id,
		OriginalURL: originalURL,
	}

	if err := r.append(rec); err != nil {
		return err
	}

	r.index[id] = originalURL
	r.count++

	return nil
}

func (r *FileRepository) Get(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	originalURL, ok := r.index[id]
	return originalURL, ok
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

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
	UserID      string `json:"user_id,omitempty"`
	IsDeleted   bool   `json:"is_deleted,omitempty"`
}

type FileRepository struct {
	mu      sync.Mutex
	index   map[string]string
	byURL   map[string]string
	byUser  map[string][]string
	owner   map[string]string
	deleted map[string]struct{}
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
		index:   make(map[string]string),
		byURL:   make(map[string]string),
		byUser:  make(map[string][]string),
		owner:   make(map[string]string),
		deleted: make(map[string]struct{}),
		path:    path,
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

		r.indexRecord(rec)
		r.count++
	}
}

func (r *FileRepository) Save(_ context.Context, rec URLRecord) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existingID, exists := r.byURL[rec.OriginalURL]; exists {
		return existingID, ErrURLConflict
	}
	if _, exists := r.index[rec.ID]; exists {
		return "", ErrIDConflict
	}

	if err := r.store(rec); err != nil {
		return "", err
	}

	return rec.ID, nil
}

func (r *FileRepository) SaveBatch(_ context.Context, records []URLRecord) ([]URLRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored := make([]URLRecord, len(records))
	fresh := make([]int, 0, len(records))
	seen := make(map[string]struct{}, len(records))

	for i, rec := range records {
		if existingID, exists := r.byURL[rec.OriginalURL]; exists {
			stored[i] = URLRecord{ID: existingID, OriginalURL: rec.OriginalURL, UserID: rec.UserID}
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
		if err := r.store(stored[i]); err != nil {
			return nil, err
		}
	}

	return stored, nil
}

func (r *FileRepository) store(rec URLRecord) error {
	line := record{
		UUID:        strconv.Itoa(r.count + 1),
		ShortURL:    rec.ID,
		OriginalURL: rec.OriginalURL,
		UserID:      rec.UserID,
	}

	if err := r.append(line); err != nil {
		return err
	}

	r.indexRecord(line)
	r.count++

	return nil
}

func (r *FileRepository) indexRecord(rec record) {
	if rec.IsDeleted {
		r.deleted[rec.ShortURL] = struct{}{}

		if originalURL, ok := r.index[rec.ShortURL]; ok {
			if r.byURL[originalURL] == rec.ShortURL {
				delete(r.byURL, originalURL)
			}
			delete(r.index, rec.ShortURL)
		}

		return
	}

	r.index[rec.ShortURL] = rec.OriginalURL
	r.byURL[rec.OriginalURL] = rec.ShortURL
	if rec.UserID != "" {
		r.byUser[rec.UserID] = append(r.byUser[rec.UserID], rec.ShortURL)
		r.owner[rec.ShortURL] = rec.UserID
	}
}

func (r *FileRepository) Get(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, deleted := r.deleted[id]; deleted {
		return "", ErrDeleted
	}

	originalURL, ok := r.index[id]
	if !ok {
		return "", ErrNotFound
	}

	return originalURL, nil
}

func (r *FileRepository) GetByUser(_ context.Context, userID string) ([]URLRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := r.byUser[userID]
	records := make([]URLRecord, 0, len(ids))
	for _, id := range ids {
		if _, deleted := r.deleted[id]; deleted {
			continue
		}
		records = append(records, URLRecord{ID: id, OriginalURL: r.index[id], UserID: userID})
	}

	return records, nil
}

func (r *FileRepository) MarkDeleted(_ context.Context, records []DeleteRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rec := range records {
		owner, ok := r.owner[rec.ID]
		if !ok || owner != rec.UserID {
			continue
		}
		if _, deleted := r.deleted[rec.ID]; deleted {
			continue
		}

		line := record{
			UUID:      strconv.Itoa(r.count + 1),
			ShortURL:  rec.ID,
			UserID:    rec.UserID,
			IsDeleted: true,
		}

		if err := r.append(line); err != nil {
			return err
		}

		r.indexRecord(line)
		r.count++
	}

	return nil
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

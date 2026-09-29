package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	yandexURL    = "http://yandex.ru"
	practicumURL = "http://practicum.yandex.ru"
)

func newRepoPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "urls.json")
}

func mustNewFileRepository(t *testing.T, path string) *FileRepository {
	t.Helper()

	repo, err := NewFileRepository(path)
	if err != nil {
		t.Fatalf("NewFileRepository failed: %v", err)
	}

	return repo
}

func readRecords(t *testing.T, path string) []record {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read storage file: %v", err)
	}

	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	records := make([]record, 0, len(lines))
	for i, line := range lines {
		var rec record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d is not a valid JSON object: %v\nline: %s", i+1, err, line)
		}

		records = append(records, rec)
	}

	return records
}

func TestFileRepository_WritesExpectedFormat(t *testing.T) {
	path := newRepoPath(t)
	repo := mustNewFileRepository(t, path)

	if err := repo.Save("4rSPg8ap", yandexURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := repo.Save("dG56Hqxm", practicumURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	want := []record{
		{UUID: "1", ShortURL: "4rSPg8ap", OriginalURL: yandexURL},
		{UUID: "2", ShortURL: "dG56Hqxm", OriginalURL: practicumURL},
	}

	got := readRecords(t, path)
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d: %+v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFileRepository_RestoresAfterRestart(t *testing.T) {
	path := newRepoPath(t)

	first := mustNewFileRepository(t, path)
	if err := first.Save("4rSPg8ap", yandexURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	restarted := mustNewFileRepository(t, path)

	originalURL, ok := restarted.Get("4rSPg8ap")
	if !ok {
		t.Fatal("Get returned ok = false, want the URL restored from file")
	}
	if originalURL != yandexURL {
		t.Errorf("Get = %q, want %q", originalURL, yandexURL)
	}
}

func TestFileRepository_ContinuesUUIDsAfterRestart(t *testing.T) {
	path := newRepoPath(t)

	first := mustNewFileRepository(t, path)
	if err := first.Save("4rSPg8ap", yandexURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	restarted := mustNewFileRepository(t, path)
	if err := restarted.Save("dG56Hqxm", practicumURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got := readRecords(t, path)
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(got), got)
	}

	if got[1].UUID != "2" {
		t.Errorf("second record uuid = %q, want %q", got[1].UUID, "2")
	}
}

func TestFileRepository_ReturnsConflictOnDuplicateID(t *testing.T) {
	repo := mustNewFileRepository(t, newRepoPath(t))

	if err := repo.Save("4rSPg8ap", yandexURL); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	err := repo.Save("4rSPg8ap", practicumURL)
	if !errors.Is(err, ErrIDConflict) {
		t.Fatalf("Save error = %v, want %v", err, ErrIDConflict)
	}
}

func TestFileRepository_StartsEmptyWhenFileMissing(t *testing.T) {
	repo := mustNewFileRepository(t, newRepoPath(t))

	if _, ok := repo.Get("4rSPg8ap"); ok {
		t.Error("Get returned ok = true on a fresh repository, want false")
	}
}

func TestFileRepository_StartsEmptyWhenFileIsEmpty(t *testing.T) {
	path := newRepoPath(t)
	if err := os.WriteFile(path, nil, storageFilePerm); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	repo := mustNewFileRepository(t, path)

	if _, ok := repo.Get("4rSPg8ap"); ok {
		t.Error("Get returned ok = true on an empty storage file, want false")
	}
}

func TestFileRepository_FailsOnMalformedFile(t *testing.T) {
	path := newRepoPath(t)
	if err := os.WriteFile(path, []byte("{not json"), storageFilePerm); err != nil {
		t.Fatalf("failed to write malformed file: %v", err)
	}

	if _, err := NewFileRepository(path); err == nil {
		t.Fatal("NewFileRepository returned nil error on a malformed file, want an error")
	}
}

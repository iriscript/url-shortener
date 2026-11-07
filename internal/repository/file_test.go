package repository

import (
	"context"
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
	t.Cleanup(func() { _ = repo.Close() })

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

	if _, err := repo.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := repo.Save(context.Background(), URLRecord{ID: "dG56Hqxm", OriginalURL: practicumURL}); err != nil {
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
	if _, err := first.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	restarted := mustNewFileRepository(t, path)

	originalURL, err := restarted.Get(context.Background(), "4rSPg8ap")
	if err != nil {
		t.Fatalf("Get failed: %v, want the URL restored from file", err)
	}
	if originalURL != yandexURL {
		t.Errorf("Get = %q, want %q", originalURL, yandexURL)
	}
}

func TestFileRepository_ContinuesUUIDsAfterRestart(t *testing.T) {
	path := newRepoPath(t)

	first := mustNewFileRepository(t, path)
	if _, err := first.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	restarted := mustNewFileRepository(t, path)
	if _, err := restarted.Save(context.Background(), URLRecord{ID: "dG56Hqxm", OriginalURL: practicumURL}); err != nil {
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

	if _, err := repo.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	_, err := repo.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: practicumURL})
	if !errors.Is(err, ErrIDConflict) {
		t.Fatalf("Save error = %v, want %v", err, ErrIDConflict)
	}
}

func TestFileRepository_StartsEmptyWhenFileMissing(t *testing.T) {
	repo := mustNewFileRepository(t, newRepoPath(t))

	if _, err := repo.Get(context.Background(), "4rSPg8ap"); !errors.Is(err, ErrNotFound) {
		t.Error("Get did not return ErrNotFound on a fresh repository")
	}
}

func TestFileRepository_StartsEmptyWhenFileIsEmpty(t *testing.T) {
	path := newRepoPath(t)
	if err := os.WriteFile(path, nil, storageFilePerm); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	repo := mustNewFileRepository(t, path)

	if _, err := repo.Get(context.Background(), "4rSPg8ap"); !errors.Is(err, ErrNotFound) {
		t.Error("Get did not return ErrNotFound on an empty storage file")
	}
}

func TestFileRepository_IsIdempotentOnRepeatedClose(t *testing.T) {
	repo := mustNewFileRepository(t, newRepoPath(t))

	if err := repo.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestFileRepository_RejectsSaveAfterClose(t *testing.T) {
	repo := mustNewFileRepository(t, newRepoPath(t))

	if err := repo.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	_, err := repo.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Save error = %v, want %v", err, ErrClosed)
	}
}

func TestFileRepository_KeepsRecordsWrittenBeforeClose(t *testing.T) {
	path := newRepoPath(t)
	repo := mustNewFileRepository(t, path)

	if _, err := repo.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	got := readRecords(t, path)
	want := []record{{UUID: "1", ShortURL: "4rSPg8ap", OriginalURL: yandexURL}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("records = %+v, want %+v", got, want)
	}
}

func TestFileRepository_PersistsUserID(t *testing.T) {
	path := newRepoPath(t)

	first := mustNewFileRepository(t, path)
	if _, err := first.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL, UserID: "alice"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := first.Save(context.Background(), URLRecord{ID: "dG56Hqxm", OriginalURL: practicumURL, UserID: "bob"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	got := readRecords(t, path)
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(got), got)
	}
	if got[0].UserID != "alice" || got[1].UserID != "bob" {
		t.Errorf("user ids = %q, %q, want %q, %q", got[0].UserID, got[1].UserID, "alice", "bob")
	}

	restarted := mustNewFileRepository(t, path)

	records, err := restarted.GetByUser(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	want := []URLRecord{{ID: "4rSPg8ap", OriginalURL: yandexURL, UserID: "alice"}}
	if len(records) != len(want) || records[0] != want[0] {
		t.Errorf("GetByUser = %+v, want %+v", records, want)
	}
}

func TestFileRepository_PersistsDeletionAcrossRestart(t *testing.T) {
	path := newRepoPath(t)

	first := mustNewFileRepository(t, path)
	if _, err := first.Save(context.Background(), URLRecord{ID: "4rSPg8ap", OriginalURL: yandexURL, UserID: "alice"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := first.MarkDeleted(context.Background(), []DeleteRecord{{ID: "4rSPg8ap", UserID: "alice"}}); err != nil {
		t.Fatalf("MarkDeleted failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	got := readRecords(t, path)
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2 (the original line plus a tombstone): %+v", len(got), got)
	}
	if !got[1].IsDeleted {
		t.Errorf("second record = %+v, want IsDeleted = true", got[1])
	}

	restarted := mustNewFileRepository(t, path)

	if _, err := restarted.Get(context.Background(), "4rSPg8ap"); !errors.Is(err, ErrDeleted) {
		t.Fatalf("Get error = %v, want %v", err, ErrDeleted)
	}
}

func TestFileRepository_LoadsRecordsWithoutUserID(t *testing.T) {
	path := newRepoPath(t)
	legacy := `{"uuid":"1","short_url":"4rSPg8ap","original_url":"` + yandexURL + `"}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), storageFilePerm); err != nil {
		t.Fatalf("failed to write legacy file: %v", err)
	}

	repo := mustNewFileRepository(t, path)

	originalURL, err := repo.Get(context.Background(), "4rSPg8ap")
	if err != nil || originalURL != yandexURL {
		t.Fatalf("Get = (%q, %v), want (%q, nil)", originalURL, err, yandexURL)
	}

	records, err := repo.GetByUser(context.Background(), "")
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("GetByUser for the empty user = %+v, want no records", records)
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

package handler_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/iriscript/url-shortener/internal/model"
	"github.com/iriscript/url-shortener/internal/repository"
)

func TestURLHandler_ShortenBatch(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid batch",
			body:       `[{"correlation_id":"1","original_url":"https://practicum.yandex.ru/"},{"correlation_id":"2","original_url":"https://ya.ru/"}]`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "empty batch",
			body:       `[]`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty url in an item",
			body:       `[{"correlation_id":"1","original_url":""}]`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed json",
			body:       `[{`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestServer(t, repository.NewMemoryRepository())

			resp, err := client.R().
				SetHeader("Content-Type", "application/json").
				SetBody(tt.body).
				Post("/api/shorten/batch")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode() != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			if ct := resp.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
			}
		})
	}
}

func TestURLHandler_ShortenBatch_PreservesCorrelationIDsAndOrder(t *testing.T) {
	request := []model.BatchRequestItem{
		{CorrelationID: "first", OriginalURL: "https://practicum.yandex.ru/"},
		{CorrelationID: "second", OriginalURL: "https://ya.ru/"},
		{CorrelationID: "third", OriginalURL: "https://go.dev/"},
	}

	client := newTestServer(t, repository.NewMemoryRepository())

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		Post("/api/shorten/batch")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusCreated)
	}

	var got []model.BatchResponseItem
	if err := json.Unmarshal(resp.Body(), &got); err != nil {
		t.Fatalf("failed to decode response %q: %v", resp.String(), err)
	}

	if len(got) != len(request) {
		t.Fatalf("got %d items, want %d", len(got), len(request))
	}

	seen := make(map[string]struct{}, len(got))
	for i, item := range got {
		if item.CorrelationID != request[i].CorrelationID {
			t.Errorf("item %d correlation_id = %q, want %q", i, item.CorrelationID, request[i].CorrelationID)
		}
		if !shortURLPattern.MatchString(item.ShortURL) {
			t.Errorf("item %d short_url = %q, want match of %q", i, item.ShortURL, shortURLPattern.String())
		}
		if _, duplicate := seen[item.ShortURL]; duplicate {
			t.Errorf("short_url %q was handed out twice", item.ShortURL)
		}
		seen[item.ShortURL] = struct{}{}
	}
}

func TestURLHandler_ShortenBatch_SavedURLsAreRedirectable(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody([]model.BatchRequestItem{{CorrelationID: "1", OriginalURL: originalURL}}).
		Post("/api/shorten/batch")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	var got []model.BatchResponseItem
	if err := json.Unmarshal(resp.Body(), &got); err != nil {
		t.Fatalf("failed to decode response %q: %v", resp.String(), err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}

	id := got[0].ShortURL[len(baseURL)+1:]

	redirect, err := client.R().Get("/" + id)
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}

	if redirect.StatusCode() != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", redirect.StatusCode(), http.StatusTemporaryRedirect)
	}
	if loc := redirect.Header().Get("Location"); loc != originalURL {
		t.Errorf("Location = %q, want %q", loc, originalURL)
	}
}

func TestURLHandler_ShortenBatch_SavesInOneCall(t *testing.T) {
	var batches [][]repository.URLRecord
	mock := &mockRepository{
		saveBatchFunc: func(_ context.Context, records []repository.URLRecord) ([]repository.URLRecord, error) {
			batches = append(batches, records)
			return records, nil
		},
		saveFunc: func(_ context.Context, rec repository.URLRecord) (string, error) {
			t.Error("ShortenBatch used Save instead of SaveBatch")
			return rec.ID, nil
		},
	}

	client := newTestServer(t, mock)

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody([]model.BatchRequestItem{
			{CorrelationID: "1", OriginalURL: "https://practicum.yandex.ru/"},
			{CorrelationID: "2", OriginalURL: "https://ya.ru/"},
		}).
		Post("/api/shorten/batch")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusCreated)
	}

	if len(batches) != 1 {
		t.Fatalf("SaveBatch called %d times, want 1", len(batches))
	}
	if len(batches[0]) != 2 {
		t.Errorf("SaveBatch got %d records, want 2", len(batches[0]))
	}
}

func TestURLHandler_ShortenBatch_ReturnsInternalErrorWhenIDsExhausted(t *testing.T) {
	mock := &mockRepository{
		saveBatchFunc: func(context.Context, []repository.URLRecord) ([]repository.URLRecord, error) {
			return nil, repository.ErrIDConflict
		},
	}

	client := newTestServer(t, mock)

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody([]model.BatchRequestItem{{CorrelationID: "1", OriginalURL: "https://practicum.yandex.ru/"}}).
		Post("/api/shorten/batch")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusInternalServerError)
	}
}

func TestURLHandler_ShortenBatch_AcceptsGzippedRequest(t *testing.T) {
	body := `[{"correlation_id":"1","original_url":"https://practicum.yandex.ru/"}]`

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte(body)); err != nil {
		t.Fatalf("failed to compress the body: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close the gzip writer: %v", err)
	}

	ts := newTestHTTPServer(t, repository.NewMemoryRepository())

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/shorten/batch", &compressed)
	if err != nil {
		t.Fatalf("failed to build the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", enc, "gzip")
	}

	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("failed to read the gzipped response: %v", err)
	}
	defer func() { _ = zr.Close() }()

	var got []model.BatchResponseItem
	if err := json.NewDecoder(zr).Decode(&got); err != nil {
		t.Fatalf("failed to decode the response: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	if got[0].CorrelationID != "1" {
		t.Errorf("correlation_id = %q, want %q", got[0].CorrelationID, "1")
	}
	if !shortURLPattern.MatchString(got[0].ShortURL) {
		t.Errorf("short_url = %q, want match of %q", got[0].ShortURL, shortURLPattern.String())
	}
}

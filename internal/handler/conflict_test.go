package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/iriscript/url-shortener/internal/model"
	"github.com/iriscript/url-shortener/internal/repository"
)

func TestURLHandler_Shorten_ReturnsConflictForKnownURL(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	first, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(originalURL).
		Post("/")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if first.StatusCode() != http.StatusCreated {
		t.Fatalf("first status = %d, want %d", first.StatusCode(), http.StatusCreated)
	}

	second, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(originalURL).
		Post("/")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	if second.StatusCode() != http.StatusConflict {
		t.Fatalf("second status = %d, want %d", second.StatusCode(), http.StatusConflict)
	}
	if got, want := second.String(), first.String(); got != want {
		t.Errorf("conflict body = %q, want the already stored %q", got, want)
	}
	if ct := second.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/plain")
	}
}

func TestURLHandler_ShortenJSON_ReturnsConflictForKnownURL(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	decode := func(t *testing.T, body []byte) model.ShortenResponse {
		t.Helper()

		var got model.ShortenResponse
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("failed to decode response %q: %v", body, err)
		}

		return got
	}

	first, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(model.ShortenRequest{URL: originalURL}).
		Post("/api/shorten")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if first.StatusCode() != http.StatusCreated {
		t.Fatalf("first status = %d, want %d", first.StatusCode(), http.StatusCreated)
	}

	second, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(model.ShortenRequest{URL: originalURL}).
		Post("/api/shorten")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	if second.StatusCode() != http.StatusConflict {
		t.Fatalf("second status = %d, want %d", second.StatusCode(), http.StatusConflict)
	}
	if ct := second.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}

	if got, want := decode(t, second.Body()).Result, decode(t, first.Body()).Result; got != want {
		t.Errorf("conflict result = %q, want the already stored %q", got, want)
	}
}

// A 409 still has to hand back a usable short URL, not just the status.
func TestURLHandler_Shorten_ConflictBodyStaysRedirectable(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	for range 2 {
		if _, err := client.R().SetBody(originalURL).Post("/"); err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}

	conflict, err := client.R().SetBody(originalURL).Post("/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if conflict.StatusCode() != http.StatusConflict {
		t.Fatalf("status = %d, want %d", conflict.StatusCode(), http.StatusConflict)
	}

	id := conflict.String()[len(baseURL)+1:]

	redirect, err := client.R().Get("/" + id)
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}
	if loc := redirect.Header().Get("Location"); loc != originalURL {
		t.Errorf("Location = %q, want %q", loc, originalURL)
	}
}

func TestURLHandler_ShortenBatch_ReusesShortURLForKnownURL(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	first, err := client.R().SetBody(originalURL).Post("/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody([]model.BatchRequestItem{{CorrelationID: "1", OriginalURL: originalURL}}).
		Post("/api/shorten/batch")
	if err != nil {
		t.Fatalf("batch request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusCreated)
	}

	var got []model.BatchResponseItem
	if err := json.Unmarshal(resp.Body(), &got); err != nil {
		t.Fatalf("failed to decode response %q: %v", resp.String(), err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}

	if got[0].ShortURL != first.String() {
		t.Errorf("short_url = %q, want the already stored %q", got[0].ShortURL, first.String())
	}
}

func TestURLHandler_ShortenBatch_CollapsesRepeatedURLs(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody([]model.BatchRequestItem{
			{CorrelationID: "1", OriginalURL: originalURL},
			{CorrelationID: "2", OriginalURL: "https://ya.ru/"},
			{CorrelationID: "3", OriginalURL: originalURL},
		}).
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
	if len(got) != 3 {
		t.Fatalf("got %d items, want 3", len(got))
	}

	if got[0].ShortURL != got[2].ShortURL {
		t.Errorf("repeated URL got %q and %q, want the same short URL", got[0].ShortURL, got[2].ShortURL)
	}
	if got[1].ShortURL == got[0].ShortURL {
		t.Error("a different URL got the same short URL")
	}
}

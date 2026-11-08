package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/iriscript/url-shortener/internal/model"
	"github.com/iriscript/url-shortener/internal/repository"
)

func TestURLHandler_UserURLs(t *testing.T) {
	tests := []struct {
		name       string
		records    []repository.URLRecord
		err        error
		wantStatus int
		wantBody   []model.UserURLItem
	}{
		{
			name: "urls of the user",
			records: []repository.URLRecord{
				{ID: "4rSPg8ap", OriginalURL: "https://practicum.yandex.ru/"},
				{ID: "dG56Hqxm", OriginalURL: "https://ya.ru/"},
			},
			wantStatus: http.StatusOK,
			wantBody: []model.UserURLItem{
				{ShortURL: baseURL + "/4rSPg8ap", OriginalURL: "https://practicum.yandex.ru/"},
				{ShortURL: baseURL + "/dG56Hqxm", OriginalURL: "https://ya.ru/"},
			},
		},
		{
			name:       "user has no urls",
			records:    nil,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "repository fails",
			err:        errors.New("storage is unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestServer(t, &mockRepository{
				getFunc: func(context.Context, string) (string, error) { return "", repository.ErrNotFound },
				getByUserFunc: func(_ context.Context, userID string) ([]repository.URLRecord, error) {
					if userID == "" {
						t.Error("handler asked the repository for an empty user id")
					}

					return tt.records, tt.err
				},
			})

			resp, err := client.R().Get("/api/user/urls")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode() != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode(), tt.wantStatus, resp.String())
			}
			if tt.wantBody == nil {
				return
			}

			if ct := resp.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
			}

			var got []model.UserURLItem
			if err := json.Unmarshal(resp.Body(), &got); err != nil {
				t.Fatalf("failed to decode the response %q: %v", resp.String(), err)
			}
			if len(got) != len(tt.wantBody) {
				t.Fatalf("got %d items, want %d: %+v", len(got), len(tt.wantBody), got)
			}
			for i := range tt.wantBody {
				if got[i] != tt.wantBody[i] {
					t.Errorf("item %d = %+v, want %+v", i, got[i], tt.wantBody[i])
				}
			}
		})
	}
}

func TestURLHandler_UserURLs_RejectsCookieWithoutUserID(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "not a token", value: "someUserID"},
		{name: "signed with another secret", value: mustToken(t, "someUserID", "another-secret", time.Hour)},
		{name: "expired", value: mustToken(t, "someUserID", testSecret, -time.Hour)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestServer(t, &mockRepository{
				getFunc: func(context.Context, string) (string, error) { return "", repository.ErrNotFound },
				getByUserFunc: func(context.Context, string) ([]repository.URLRecord, error) {
					t.Error("handler queried the repository for an unauthorized request")
					return nil, nil
				},
			})

			resp, err := client.R().SetCookie(&http.Cookie{Name: "auth", Value: tt.value}).Get("/api/user/urls")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode() != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode(), http.StatusUnauthorized, resp.String())
			}
		})
	}
}

func TestURLHandler_UserURLs_ReturnsOwnShortenedURL(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	created, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(originalURL).
		Post("/")
	if err != nil {
		t.Fatalf("POST / failed: %v", err)
	}
	if created.StatusCode() != http.StatusCreated {
		t.Fatalf("POST / status = %d, want %d", created.StatusCode(), http.StatusCreated)
	}

	resp, err := client.R().Get("/api/user/urls")
	if err != nil {
		t.Fatalf("GET /api/user/urls failed: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode(), http.StatusOK, resp.String())
	}

	var got []model.UserURLItem
	if err := json.Unmarshal(resp.Body(), &got); err != nil {
		t.Fatalf("failed to decode the response %q: %v", resp.String(), err)
	}

	want := []model.UserURLItem{{ShortURL: created.String(), OriginalURL: originalURL}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("GET /api/user/urls = %+v, want %+v", got, want)
	}
}

func TestURLHandler_UserURLs_IsolatesUsers(t *testing.T) {
	ts := newTestHTTPServer(t, repository.NewMemoryRepository())
	owner := newTestClient(ts)
	stranger := newTestClient(ts)

	if _, err := owner.R().
		SetHeader("Content-Type", "text/plain").
		SetBody("https://practicum.yandex.ru/").
		Post("/"); err != nil {
		t.Fatalf("POST / failed: %v", err)
	}

	resp, err := stranger.R().Get("/api/user/urls")
	if err != nil {
		t.Fatalf("GET /api/user/urls failed: %v", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		t.Errorf("status = %d, want %d: %s", resp.StatusCode(), http.StatusNoContent, resp.String())
	}
}

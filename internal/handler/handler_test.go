package handler_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/go-resty/resty/v2"

	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/handler"
	"github.com/iriscript/url-shortener/internal/repository"
	"github.com/iriscript/url-shortener/internal/server"
)

const baseURL = "http://localhost:8080"

var shortURLPattern = regexp.MustCompile(`^` + regexp.QuoteMeta(baseURL) + `/[a-zA-Z0-9]{8}$`)

type mockRepository struct {
	saveFunc func(id, originalURL string) error
	getFunc  func(id string) (string, bool)
}

var _ repository.URLRepository = (*mockRepository)(nil)

func (m *mockRepository) Save(id, originalURL string) error {
	return m.saveFunc(id, originalURL)
}

func (m *mockRepository) Get(id string) (string, bool) {
	return m.getFunc(id)
}

func newTestServer(t *testing.T, repo repository.URLRepository) *resty.Client {
	t.Helper()

	router := server.NewRouter(handler.NewURLHandler(repo, config.HandlerConfig{BaseURL: baseURL}))
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return resty.New().
		SetBaseURL(ts.URL).
		SetRedirectPolicy(resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}))
}

func TestURLHandler_Shorten(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid url",
			body:       "https://practicum.yandex.ru/",
			wantStatus: http.StatusCreated,
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
				SetHeader("Content-Type", "text/plain").
				SetBody(tt.body).
				Post("/")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode() != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			if tt.wantStatus != http.StatusCreated {
				return
			}

			if ct := resp.Header().Get("Content-Type"); ct != "text/plain" {
				t.Errorf("Content-Type = %q, want %q", ct, "text/plain")
			}

			body := resp.String()
			if !shortURLPattern.MatchString(body) {
				t.Errorf("body = %q, want match of %q", body, shortURLPattern.String())
			}
		})
	}
}

func TestURLHandler_Shorten_UsesRepository(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	var gotSavedID, gotSavedURL string
	mock := &mockRepository{
		saveFunc: func(id, url string) error {
			gotSavedID = id
			gotSavedURL = url
			return nil
		},
	}

	client := newTestServer(t, mock)

	resp, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(originalURL).
		Post("/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusCreated)
	}

	if gotSavedURL != originalURL {
		t.Errorf("repo.Save called with %q, want %q", gotSavedURL, originalURL)
	}

	wantBody := baseURL + "/" + gotSavedID
	if got := resp.String(); got != wantBody {
		t.Errorf("body = %q, want %q", got, wantBody)
	}
}

func TestURLHandler_Shorten_ReturnsInternalErrorWhenIDsExhausted(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	mock := &mockRepository{
		saveFunc: func(id, url string) error {
			return repository.ErrIDConflict
		},
	}

	client := newTestServer(t, mock)

	resp, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(originalURL).
		Post("/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusInternalServerError)
	}
}

func TestURLHandler_Redirect(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"
	const knownID = "knownID1"

	tests := []struct {
		name         string
		id           string
		wantStatus   int
		wantLocation string
	}{
		{
			name:         "known id",
			id:           knownID,
			wantStatus:   http.StatusTemporaryRedirect,
			wantLocation: originalURL,
		},
		{
			name:       "unknown id",
			id:         "doesNotExist",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repository.NewMemoryRepository()
			if err := repo.Save(knownID, originalURL); err != nil {
				t.Fatalf("repo.Save failed: %v", err)
			}
			client := newTestServer(t, repo)

			resp, err := client.R().Get("/" + tt.id)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode() != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			if loc := resp.Header().Get("Location"); loc != tt.wantLocation {
				t.Errorf("Location = %q, want %q", loc, tt.wantLocation)
			}
		})
	}
}

func TestURLHandler_Redirect_UnknownIDNeverHitsRepository(t *testing.T) {
	mock := &mockRepository{
		getFunc: func(id string) (string, bool) {
			return "", false
		},
	}

	client := newTestServer(t, mock)

	resp, err := client.R().Get("/anyID123")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusBadRequest)
	}
}

func TestURLHandler_ShortenAndRedirect_RoundTrip(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	client := newTestServer(t, repository.NewMemoryRepository())

	postResp, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(originalURL).
		Post("/")
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}

	if postResp.StatusCode() != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d", postResp.StatusCode(), http.StatusCreated)
	}

	shortURL := postResp.String()
	id := shortURL[len(baseURL+"/"):]

	getResp, err := client.R().Get("/" + id)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}

	if getResp.StatusCode() != http.StatusTemporaryRedirect {
		t.Fatalf("GET status = %d, want %d", getResp.StatusCode(), http.StatusTemporaryRedirect)
	}

	if loc := getResp.Header().Get("Location"); loc != originalURL {
		t.Errorf("Location = %q, want %q", loc, originalURL)
	}
}

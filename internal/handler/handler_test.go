package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/iriscript/url-shortener/internal/handler"
	"github.com/iriscript/url-shortener/internal/repository"
	"github.com/iriscript/url-shortener/internal/server"
)

const baseURL = "http://localhost:8080"

var shortURLPattern = regexp.MustCompile(`^` + regexp.QuoteMeta(baseURL) + `/[a-zA-Z0-9]{8}$`)

type mockRepository struct {
	saveFunc func(originalURL string) string
	getFunc  func(id string) (string, bool)
}

var _ repository.URLRepository = (*mockRepository)(nil)

func (m *mockRepository) Save(originalURL string) string {
	return m.saveFunc(originalURL)
}

func (m *mockRepository) Get(id string) (string, bool) {
	return m.getFunc(id)
}

func closeBody(t *testing.T, body io.ReadCloser) {
	t.Helper()
	if err := body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}
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
			router := server.NewRouter(handler.NewURLHandler(repository.NewMemoryRepository(), baseURL))

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)
			res := rec.Result()
			defer closeBody(t, res.Body)

			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if tt.wantStatus != http.StatusCreated {
				return
			}

			if ct := res.Header.Get("Content-Type"); ct != "text/plain" {
				t.Errorf("Content-Type = %q, want %q", ct, "text/plain")
			}

			body := rec.Body.String()
			if !shortURLPattern.MatchString(body) {
				t.Errorf("body = %q, want match of %q", body, shortURLPattern.String())
			}
		})
	}
}

func TestURLHandler_Shorten_UsesRepository(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"
	const fixedID = "fixedID1"

	var gotSavedURL string
	mock := &mockRepository{
		saveFunc: func(url string) string {
			gotSavedURL = url
			return fixedID
		},
	}

	router := server.NewRouter(handler.NewURLHandler(mock, baseURL))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(originalURL))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	if gotSavedURL != originalURL {
		t.Errorf("repo.Save called with %q, want %q", gotSavedURL, originalURL)
	}

	wantBody := baseURL + "/" + fixedID
	if got := rec.Body.String(); got != wantBody {
		t.Errorf("body = %q, want %q", got, wantBody)
	}
}

func TestURLHandler_Redirect(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	repo := repository.NewMemoryRepository()
	knownID := repo.Save(originalURL)
	router := server.NewRouter(handler.NewURLHandler(repo, baseURL))

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
			req := httptest.NewRequest(http.MethodGet, "/"+tt.id, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)
			res := rec.Result()
			defer closeBody(t, res.Body)

			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if loc := res.Header.Get("Location"); loc != tt.wantLocation {
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

	router := server.NewRouter(handler.NewURLHandler(mock, baseURL))

	req := httptest.NewRequest(http.MethodGet, "/anyID123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestURLHandler_ShortenAndRedirect_RoundTrip(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	router := server.NewRouter(handler.NewURLHandler(repository.NewMemoryRepository(), baseURL))

	postReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(originalURL))
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d", postRec.Code, http.StatusCreated)
	}

	shortURL := postRec.Body.String()
	id := strings.TrimPrefix(shortURL, baseURL+"/")

	getReq := httptest.NewRequest(http.MethodGet, "/"+id, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("GET status = %d, want %d", getRec.Code, http.StatusTemporaryRedirect)
	}

	if loc := getRec.Header().Get("Location"); loc != originalURL {
		t.Errorf("Location = %q, want %q", loc, originalURL)
	}
}

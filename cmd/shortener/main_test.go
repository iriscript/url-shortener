package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var shortURLPattern = regexp.MustCompile(`^http://` + regexp.QuoteMeta(serverAddr) + `/[a-zA-Z0-9]{8}$`)

func TestShortenHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantContentTyp string
	}{
		{
			name:           "valid url",
			body:           "https://practicum.yandex.ru/",
			wantStatus:     http.StatusCreated,
			wantContentTyp: "text/plain",
		},
		{
			name:       "empty body",
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRouter(newURLStore())

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)
			res := rec.Result()
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(res.Body)

			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if tt.wantStatus != http.StatusCreated {
				return
			}

			if ct := res.Header.Get("Content-Type"); ct != tt.wantContentTyp {
				t.Errorf("Content-Type = %q, want %q", ct, tt.wantContentTyp)
			}

			body := rec.Body.String()
			if !shortURLPattern.MatchString(body) {
				t.Errorf("body = %q, want match of %q", body, shortURLPattern.String())
			}
		})
	}
}

func TestRedirectHandler(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	store := newURLStore()
	knownID := store.Save(originalURL)
	router := newRouter(store)

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
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(res.Body)

			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}

			if loc := res.Header.Get("Location"); loc != tt.wantLocation {
				t.Errorf("Location = %q, want %q", loc, tt.wantLocation)
			}
		})
	}
}

func TestShortenAndRedirect_RoundTrip(t *testing.T) {
	const originalURL = "https://practicum.yandex.ru/"

	router := newRouter(newURLStore())

	postReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(originalURL))
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d", postRec.Code, http.StatusCreated)
	}

	shortURL := postRec.Body.String()
	id := strings.TrimPrefix(shortURL, "http://"+serverAddr+"/")

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

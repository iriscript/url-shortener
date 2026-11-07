package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iriscript/url-shortener/internal/handler"
	"github.com/iriscript/url-shortener/internal/repository"
	"github.com/iriscript/url-shortener/internal/router"

	"github.com/iriscript/url-shortener/internal/config"
)

type mockPinger struct {
	pingFunc func(ctx context.Context) error
}

var _ handler.Pinger = (*mockPinger)(nil)

func (m *mockPinger) Ping(ctx context.Context) error {
	return m.pingFunc(ctx)
}

func newTestPingServer(t *testing.T, pinger handler.Pinger) *httptest.Server {
	t.Helper()

	repo := &mockRepository{
		saveFunc: func(_ context.Context, rec repository.URLRecord) (string, error) { return rec.ID, nil },
		getFunc:  func(context.Context, string) (string, error) { return "", repository.ErrNotFound },
	}

	rout := router.NewRouter(
		handler.NewURLHandler(repo, &mockDeleter{}, config.HandlerConfig{BaseURL: baseURL}),
		handler.NewPingHandler(pinger),
		config.AuthConfig{SecretKey: testSecret},
	)
	ts := httptest.NewServer(rout)
	t.Cleanup(ts.Close)

	return ts
}

func TestPing(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
	}{
		{
			name:       "database is available",
			pingErr:    nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "database is unavailable",
			pingErr:    errors.New("connection refused"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			ts := newTestPingServer(t, &mockPinger{
				pingFunc: func(context.Context) error {
					called = true
					return tt.pingErr
				},
			})

			resp, err := http.Get(ts.URL + "/ping")
			if err != nil {
				t.Fatalf("GET /ping failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if !called {
				t.Error("handler did not call Ping")
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestPing_NoDatabaseConfigured(t *testing.T) {
	ts := newTestPingServer(t, nil)

	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("GET /ping failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

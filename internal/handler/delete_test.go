package handler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/iriscript/url-shortener/internal/auth"
	"github.com/iriscript/url-shortener/internal/deleter"
	"github.com/iriscript/url-shortener/internal/repository"
)

func TestURLHandler_DeleteUserURLs_RejectsCookieWithoutUserID(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "not signed", value: "someUserID"},
		{name: "wrong signature", value: "someUserID.0f0f0f0f"},
		{name: "signed with another secret", value: auth.Sign("someUserID", []byte("another-secret"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			del := &mockDeleter{
				enqueueFunc: func(string, []string) {
					t.Error("handler enqueued a delete for an unauthorized request")
				},
			}
			client := newTestServerWithDeleter(t, &mockRepository{
				getFunc: func(context.Context, string) (string, error) { return "", repository.ErrNotFound },
			}, del)

			resp, err := client.R().
				SetCookie(&http.Cookie{Name: "auth", Value: tt.value}).
				SetHeader("Content-Type", "application/json").
				SetBody(`["id1"]`).
				Delete("/api/user/urls")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode() != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode(), http.StatusUnauthorized, resp.String())
			}
		})
	}
}

func TestURLHandler_DeleteUserURLs_EnqueuesAndReturnsAccepted(t *testing.T) {
	var gotUserID string
	var gotIDs []string

	del := &mockDeleter{
		enqueueFunc: func(userID string, ids []string) {
			gotUserID = userID
			gotIDs = ids
		},
	}
	repo := &mockRepository{
		saveFunc: func(_ context.Context, rec repository.URLRecord) (string, error) { return rec.ID, nil },
		getFunc:  func(context.Context, string) (string, error) { return "", repository.ErrNotFound },
	}
	client := newTestServerWithDeleter(t, repo, del)

	if _, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody("https://practicum.yandex.ru/").
		Post("/"); err != nil {
		t.Fatalf("POST / failed: %v", err)
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(`["6qxTVvsy", "RTfd56hn"]`).
		Delete("/api/user/urls")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode(), http.StatusAccepted, resp.String())
	}

	if gotUserID == "" {
		t.Error("deleter was not invoked with a user id")
	}

	want := []string{"6qxTVvsy", "RTfd56hn"}
	if len(gotIDs) != len(want) {
		t.Fatalf("got ids %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("ids[%d] = %q, want %q", i, gotIDs[i], want[i])
		}
	}
}

func TestURLHandler_DeleteUserURLs_InvalidBody(t *testing.T) {
	del := &mockDeleter{
		enqueueFunc: func(string, []string) {
			t.Error("handler enqueued a delete for an invalid request body")
		},
	}
	repo := &mockRepository{
		saveFunc: func(_ context.Context, rec repository.URLRecord) (string, error) { return rec.ID, nil },
		getFunc:  func(context.Context, string) (string, error) { return "", repository.ErrNotFound },
	}
	client := newTestServerWithDeleter(t, repo, del)

	if _, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody("https://practicum.yandex.ru/").
		Post("/"); err != nil {
		t.Fatalf("POST / failed: %v", err)
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(`not a json array`).
		Delete("/api/user/urls")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode(), http.StatusBadRequest, resp.String())
	}
}

func TestURLHandler_DeleteUserURLs_EmptyArray(t *testing.T) {
	del := &mockDeleter{
		enqueueFunc: func(string, []string) {
			t.Error("handler enqueued a delete for an empty id list")
		},
	}
	repo := &mockRepository{
		saveFunc: func(_ context.Context, rec repository.URLRecord) (string, error) { return rec.ID, nil },
		getFunc:  func(context.Context, string) (string, error) { return "", repository.ErrNotFound },
	}
	client := newTestServerWithDeleter(t, repo, del)

	if _, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody("https://practicum.yandex.ru/").
		Post("/"); err != nil {
		t.Fatalf("POST / failed: %v", err)
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(`[]`).
		Delete("/api/user/urls")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode(), http.StatusAccepted, resp.String())
	}
}

func TestURLHandler_DeleteUserURLs_RoundTrip(t *testing.T) {
	repo := repository.NewMemoryRepository()
	del := deleter.New(repo)

	client := newTestServerWithDeleter(t, repo, del)

	created, err := client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody("https://practicum.yandex.ru/").
		Post("/")
	if err != nil {
		t.Fatalf("POST / failed: %v", err)
	}
	if created.StatusCode() != http.StatusCreated {
		t.Fatalf("POST / status = %d, want %d", created.StatusCode(), http.StatusCreated)
	}

	id := created.String()[len(baseURL+"/"):]

	delResp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(`["` + id + `"]`).
		Delete("/api/user/urls")
	if err != nil {
		t.Fatalf("DELETE /api/user/urls failed: %v", err)
	}
	if delResp.StatusCode() != http.StatusAccepted {
		t.Fatalf("DELETE status = %d, want %d", delResp.StatusCode(), http.StatusAccepted)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := del.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	getResp, err := client.R().Get("/" + id)
	if err != nil {
		t.Fatalf("GET /%s failed: %v", id, err)
	}
	if getResp.StatusCode() != http.StatusGone {
		t.Fatalf("GET status = %d, want %d: %s", getResp.StatusCode(), http.StatusGone, getResp.String())
	}
}

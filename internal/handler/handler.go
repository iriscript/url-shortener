package handler

import (
	"io"
	"log"
	"net/http"

	"github.com/iriscript/url-shortener/internal/repository"
)

type URLHandler struct {
	repo    repository.URLRepository
	baseURL string
}

func NewURLHandler(repo repository.URLRepository, baseURL string) *URLHandler {
	return &URLHandler{repo: repo, baseURL: baseURL}
}

func (h *URLHandler) Shorten(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "empty or invalid request body", http.StatusBadRequest)
		return
	}

	id := h.repo.Save(string(body))

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte(h.baseURL + "/" + id)); err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

func (h *URLHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	originalURL, ok := h.repo.Get(id)
	if !ok {
		http.Error(w, "unknown short URL id", http.StatusBadRequest)
		return
	}

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

package handler

import (
	"errors"
	"log"
	"math/rand"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/repository"
)

const (
	idAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	idLength   = 8

	maxSaveAttempts = 100
)

type URLRepository interface {
	Save(id, originalURL string) error
	Get(id string) (originalURL string, ok bool)
}

type URLHandler struct {
	repo    URLRepository
	baseURL string
}

func NewURLHandler(repo URLRepository, cfg config.HandlerConfig) *URLHandler {
	return &URLHandler{repo: repo, baseURL: cfg.BaseURL}
}

func (h *URLHandler) Shorten(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil || len(body) == 0 {
		c.String(http.StatusBadRequest, "empty or invalid request body")
		return
	}

	id, err := h.save(string(body))
	if err != nil {
		log.Printf("shorten: failed to save url: %v", err)
		c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	shortURL, err := url.JoinPath(h.baseURL, id)
	if err != nil {
		log.Printf("shorten: failed to build short url: %v", err)
		c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	c.Data(http.StatusCreated, "text/plain", []byte(shortURL))
}

func (h *URLHandler) save(originalURL string) (string, error) {
	for attempt := 0; attempt < maxSaveAttempts; attempt++ {
		id := generateID()

		err := h.repo.Save(id, originalURL)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, repository.ErrIDConflict) {
			return "", err
		}
	}

	return "", repository.ErrIDConflict
}

func (h *URLHandler) Redirect(c *gin.Context) {
	id := c.Param("id")

	originalURL, ok := h.repo.Get(id)
	if !ok {
		c.String(http.StatusBadRequest, "unknown short URL id")
		return
	}

	c.Header("Location", originalURL)
	c.Status(http.StatusTemporaryRedirect)
}

func generateID() string {
	b := make([]byte, idLength)
	for i := range b {
		b[i] = idAlphabet[rand.Intn(len(idAlphabet))]
	}
	return string(b)
}

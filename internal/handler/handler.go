package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/iriscript/url-shortener/internal/model"
	"github.com/rs/zerolog/log"

	"github.com/iriscript/url-shortener/internal/config"
	"github.com/iriscript/url-shortener/internal/repository"
)

const (
	idAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	idLength   = 8

	maxSaveAttempts = 100

	contentTypeJSON = "application/json; charset=utf-8"
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

	shortURL, err := h.shortenURL(string(body))
	if err != nil {
		log.Error().Err(err).Msg("shorten: failed to shorten url")
		c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	c.Data(http.StatusCreated, "text/plain", []byte(shortURL))
}

func (h *URLHandler) ShortenJSON(c *gin.Context) {
	var request model.ShortenRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		log.Warn().Err(err).Msg("shortenJSON: failed to decode request body")
		writeJSON(c, http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if request.URL == "" {
		log.Warn().Msg("shortenJSON: url is empty")
		writeJSON(c, http.StatusBadRequest, gin.H{"error": "url is empty"})
		return
	}

	shortURL, err := h.shortenURL(request.URL)
	if err != nil {
		log.Error().Err(err).Msg("shortenJSON: failed to shorten url")
		writeJSON(c, http.StatusInternalServerError, gin.H{"error": http.StatusText(http.StatusInternalServerError)})
		return
	}

	writeJSON(c, http.StatusCreated, model.ShortenResponse{Result: shortURL})
}

func writeJSON(c *gin.Context, status int, response any) {
	body, err := json.Marshal(response)
	if err != nil {
		log.Error().Err(err).Msg("writeJSON: failed to marshal response")
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(status, contentTypeJSON, body)
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

func (h *URLHandler) shortenURL(originalURL string) (string, error) {
	id, err := h.save(originalURL)
	if err != nil {
		return "", err
	}

	return url.JoinPath(h.baseURL, id)
}

func (h *URLHandler) save(originalURL string) (string, error) {
	for attempt := 0; attempt < maxSaveAttempts; attempt++ {
		id := generateID()

		err := h.repo.Save(id, originalURL)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, repository.ErrIDConflict) {
			return "", fmt.Errorf("failed to save the URL: %w", err)
		}
	}

	return "", repository.ErrIDConflict
}

func generateID() string {
	b := make([]byte, idLength)
	for i := range b {
		b[i] = idAlphabet[rand.Intn(len(idAlphabet))]
	}
	return string(b)
}

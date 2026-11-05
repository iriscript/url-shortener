package handler

import (
	"context"
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
	Save(ctx context.Context, id, originalURL string) error
	SaveBatch(ctx context.Context, records []repository.URLRecord) error
	Get(ctx context.Context, id string) (originalURL string, ok bool)
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

	shortURL, err := h.shortenURL(c.Request.Context(), string(body))
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

	shortURL, err := h.shortenURL(c.Request.Context(), request.URL)
	if err != nil {
		log.Error().Err(err).Msg("shortenJSON: failed to shorten url")
		writeJSON(c, http.StatusInternalServerError, gin.H{"error": http.StatusText(http.StatusInternalServerError)})
		return
	}

	writeJSON(c, http.StatusCreated, model.ShortenResponse{Result: shortURL})
}

func (h *URLHandler) ShortenBatch(c *gin.Context) {
	var request []model.BatchRequestItem
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		log.Warn().Err(err).Msg("shortenBatch: failed to decode request body")
		writeJSON(c, http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(request) == 0 {
		log.Warn().Msg("shortenBatch: batch is empty")
		writeJSON(c, http.StatusBadRequest, gin.H{"error": "batch is empty"})
		return
	}
	for _, item := range request {
		if item.OriginalURL == "" {
			log.Warn().Str("correlationID", item.CorrelationID).Msg("shortenBatch: url is empty")
			writeJSON(c, http.StatusBadRequest, gin.H{"error": "url is empty"})
			return
		}
	}

	response, err := h.shortenBatch(c.Request.Context(), request)
	if err != nil {
		log.Error().Err(err).Msg("shortenBatch: failed to shorten the batch")
		writeJSON(c, http.StatusInternalServerError, gin.H{"error": http.StatusText(http.StatusInternalServerError)})
		return
	}

	writeJSON(c, http.StatusCreated, response)
}

func (h *URLHandler) shortenBatch(ctx context.Context, request []model.BatchRequestItem) ([]model.BatchResponseItem, error) {
	records := make([]repository.URLRecord, len(request))

	for attempt := 0; attempt < maxSaveAttempts; attempt++ {
		for i, item := range request {
			records[i] = repository.URLRecord{ID: generateID(), OriginalURL: item.OriginalURL}
		}

		err := h.repo.SaveBatch(ctx, records)
		if errors.Is(err, repository.ErrIDConflict) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to save the URL batch: %w", err)
		}

		response := make([]model.BatchResponseItem, len(request))
		for i, item := range request {
			shortURL, err := url.JoinPath(h.baseURL, records[i].ID)
			if err != nil {
				return nil, fmt.Errorf("failed to build the short URL: %w", err)
			}

			response[i] = model.BatchResponseItem{CorrelationID: item.CorrelationID, ShortURL: shortURL}
		}

		return response, nil
	}

	return nil, repository.ErrIDConflict
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

	originalURL, ok := h.repo.Get(c.Request.Context(), id)
	if !ok {
		c.String(http.StatusBadRequest, "unknown short URL id")
		return
	}

	c.Header("Location", originalURL)
	c.Status(http.StatusTemporaryRedirect)
}

func (h *URLHandler) shortenURL(ctx context.Context, originalURL string) (string, error) {
	id, err := h.save(ctx, originalURL)
	if err != nil {
		return "", err
	}

	return url.JoinPath(h.baseURL, id)
}

func (h *URLHandler) save(ctx context.Context, originalURL string) (string, error) {
	for attempt := 0; attempt < maxSaveAttempts; attempt++ {
		id := generateID()

		err := h.repo.Save(ctx, id, originalURL)
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

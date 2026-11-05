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
	Save(ctx context.Context, id, originalURL string) (storedID string, err error)
	SaveBatch(ctx context.Context, records []repository.URLRecord) (stored []repository.URLRecord, err error)
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

	shortURL, existed, err := h.shortenURL(c.Request.Context(), string(body))
	if err != nil {
		log.Error().Err(err).Msg("shorten: failed to shorten url")
		c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	status := http.StatusCreated
	if existed {
		status = http.StatusConflict
	}

	c.Data(status, "text/plain", []byte(shortURL))
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

	shortURL, existed, err := h.shortenURL(c.Request.Context(), request.URL)
	if err != nil {
		log.Error().Err(err).Msg("shortenJSON: failed to shorten url")
		writeJSON(c, http.StatusInternalServerError, gin.H{"error": http.StatusText(http.StatusInternalServerError)})
		return
	}

	status := http.StatusCreated
	if existed {
		status = http.StatusConflict
	}

	writeJSON(c, status, model.ShortenResponse{Result: shortURL})
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
	uniqueURLs := make([]string, 0, len(request))
	positions := make(map[string]int, len(request))
	for _, item := range request {
		if _, seen := positions[item.OriginalURL]; seen {
			continue
		}

		positions[item.OriginalURL] = len(uniqueURLs)
		uniqueURLs = append(uniqueURLs, item.OriginalURL)
	}

	records := make([]repository.URLRecord, len(uniqueURLs))

	for attempt := 0; attempt < maxSaveAttempts; attempt++ {
		for i, originalURL := range uniqueURLs {
			records[i] = repository.URLRecord{ID: generateID(), OriginalURL: originalURL}
		}

		stored, err := h.repo.SaveBatch(ctx, records)
		if errors.Is(err, repository.ErrIDConflict) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to save the URL batch: %w", err)
		}

		response := make([]model.BatchResponseItem, len(request))
		for i, item := range request {
			shortURL, err := url.JoinPath(h.baseURL, stored[positions[item.OriginalURL]].ID)
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

func (h *URLHandler) shortenURL(ctx context.Context, originalURL string) (string, bool, error) {
	id, existed, err := h.save(ctx, originalURL)
	if err != nil {
		return "", false, err
	}

	shortURL, err := url.JoinPath(h.baseURL, id)
	if err != nil {
		return "", false, fmt.Errorf("failed to build the short URL: %w", err)
	}

	return shortURL, existed, nil
}

func (h *URLHandler) save(ctx context.Context, originalURL string) (string, bool, error) {
	for attempt := 0; attempt < maxSaveAttempts; attempt++ {
		id := generateID()

		storedID, err := h.repo.Save(ctx, id, originalURL)
		if err == nil {
			return storedID, false, nil
		}
		if errors.Is(err, repository.ErrURLConflict) {
			return storedID, true, nil
		}
		if !errors.Is(err, repository.ErrIDConflict) {
			return "", false, fmt.Errorf("failed to save the URL: %w", err)
		}
	}

	return "", false, repository.ErrIDConflict
}

func generateID() string {
	b := make([]byte, idLength)
	for i := range b {
		b[i] = idAlphabet[rand.Intn(len(idAlphabet))]
	}
	return string(b)
}

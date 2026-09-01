package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/iriscript/url-shortener/internal/repository"
)

type URLHandler struct {
	repo    repository.URLRepository
	baseURL string
}

func NewURLHandler(repo repository.URLRepository, baseURL string) *URLHandler {
	return &URLHandler{repo: repo, baseURL: baseURL}
}

func (h *URLHandler) Shorten(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil || len(body) == 0 {
		c.String(http.StatusBadRequest, "empty or invalid request body")
		return
	}

	id := h.repo.Save(string(body))

	c.Data(http.StatusCreated, "text/plain", []byte(h.baseURL+"/"+id))
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

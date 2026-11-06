package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const pingTimeout = 3 * time.Second

type Pinger interface {
	Ping(ctx context.Context) error
}

type PingHandler struct {
	db Pinger
}

func NewPingHandler(db Pinger) *PingHandler {
	return &PingHandler{db: db}
}

func (h *PingHandler) Ping(c *gin.Context) {
	if h.db == nil {
		log.Error().Msg("ping: database is not configured")
		c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), pingTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		log.Error().Err(err).Msg("ping: database is unavailable")
		c.String(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	c.Status(http.StatusOK)
}

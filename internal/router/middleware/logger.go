package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func LogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		log.Info().
			Str("method", c.Request.Method).
			Str("uri", c.Request.RequestURI).
			Dur("latency", latency).
			Msg("Gin Request")

		log.Info().
			Int("statusCode", c.Writer.Status()).
			Int("contentLength", c.Writer.Size()).
			Msg("Gin Response")

	}
}

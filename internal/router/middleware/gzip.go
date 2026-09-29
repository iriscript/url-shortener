package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

var compressibleTypes = []string{"application/json", "text/html"}

func GzipMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.Contains(c.GetHeader("Content-Encoding"), "gzip") {
			gr, err := gzip.NewReader(c.Request.Body)
			if err != nil {
				log.Warn().Err(err).Msg("gzip: failed to read compressed request body")
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			defer func() {
				if err := gr.Close(); err != nil {
					log.Warn().Err(err).Msg("gzip: failed to close request body reader")
				}
			}()

			c.Request.Body = gr
			c.Request.Header.Del("Content-Encoding")
			c.Request.ContentLength = -1
		}

		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		cw := &compressWriter{ResponseWriter: c.Writer}
		defer cw.close()
		c.Writer = cw

		c.Next()
	}
}

type compressWriter struct {
	gin.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

func (w *compressWriter) Write(data []byte) (int, error) {
	if w.shouldCompress() {
		return w.gz.Write(data)
	}

	return w.ResponseWriter.Write(data)
}

func (w *compressWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *compressWriter) shouldCompress() bool {
	if w.decided {
		return w.gz != nil
	}
	w.decided = true

	contentType := w.Header().Get("Content-Type")
	for _, compressible := range compressibleTypes {
		if strings.HasPrefix(contentType, compressible) {
			w.Header().Del("Content-Length")
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			w.gz = gzip.NewWriter(w.ResponseWriter)

			return true
		}
	}

	return false
}

func (w *compressWriter) close() {
	if w.gz == nil {
		return
	}

	if err := w.gz.Close(); err != nil {
		log.Error().Err(err).Msg("gzip: failed to close gzip writer")
	}
}

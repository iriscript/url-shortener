package middleware_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/iriscript/url-shortener/internal/router/middleware"
)

const (
	jsonBody  = `{"result":"http://localhost:8080/EwHXdJfB"}`
	htmlBody  = `<html><body>short</body></html>`
	plainBody = `http://localhost:8080/EwHXdJfB`
)

func newGzipTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.GzipMiddleware())
	router.GET("/json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(jsonBody))
	})
	router.GET("/html", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlBody))
	})
	router.GET("/plain", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain", []byte(plainBody))
	})
	router.POST("/echo", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	})

	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return ts
}

func do(t *testing.T, method, url string, header http.Header, body []byte) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	for key, values := range header {
		req.Header[key] = values
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func mustGzip(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		t.Fatalf("failed to compress: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

func mustGunzip(t *testing.T, r io.Reader) string {
	t.Helper()

	gr, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("response body is not valid gzip: %v", err)
	}
	defer func() { _ = gr.Close() }()

	data, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to decompress response: %v", err)
	}

	return string(data)
}

func TestGzipMiddleware_CompressesByContentType(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantBody     string
		wantCompress bool
	}{
		{name: "json is compressed", path: "/json", wantBody: jsonBody, wantCompress: true},
		{name: "html is compressed", path: "/html", wantBody: htmlBody, wantCompress: true},
		{name: "plain text is not compressed", path: "/plain", wantBody: plainBody, wantCompress: false},
	}

	ts := newGzipTestServer(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{"Accept-Encoding": []string{"gzip"}}
			resp := do(t, http.MethodGet, ts.URL+tt.path, header, nil)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			enc := resp.Header.Get("Content-Encoding")
			if !tt.wantCompress {
				if enc != "" {
					t.Fatalf("Content-Encoding = %q, want empty", enc)
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				if string(body) != tt.wantBody {
					t.Errorf("body = %q, want %q", body, tt.wantBody)
				}
				return
			}

			if enc != "gzip" {
				t.Fatalf("Content-Encoding = %q, want %q", enc, "gzip")
			}

			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}

			// Content-Length должен описывать сжатое тело. Если бы обёртка не удаляла
			// заголовок, выставленный render.Data, здесь осталась бы длина исходных данных.
			if resp.ContentLength != -1 && resp.ContentLength != int64(len(raw)) {
				t.Errorf("Content-Length = %d, want %d (actual compressed size)", resp.ContentLength, len(raw))
			}

			if got := mustGunzip(t, bytes.NewReader(raw)); got != tt.wantBody {
				t.Errorf("decompressed body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestGzipMiddleware_SkipsWithoutAcceptEncoding(t *testing.T) {
	ts := newGzipTestServer(t)

	header := http.Header{"Accept-Encoding": []string{"identity"}}
	resp := do(t, http.MethodGet, ts.URL+"/json", header, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q, want empty", enc)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if string(body) != jsonBody {
		t.Errorf("body = %q, want %q", body, jsonBody)
	}
}

func TestGzipMiddleware_DecompressesRequest(t *testing.T) {
	ts := newGzipTestServer(t)

	header := http.Header{
		"Content-Type":     []string{"application/json"},
		"Content-Encoding": []string{"gzip"},
		"Accept-Encoding":  []string{"identity"},
	}
	resp := do(t, http.MethodPost, ts.URL+"/echo", header, mustGzip(t, []byte(jsonBody)))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if string(body) != jsonBody {
		t.Errorf("handler received %q, want %q", body, jsonBody)
	}
}

func TestGzipMiddleware_RejectsInvalidGzipRequest(t *testing.T) {
	ts := newGzipTestServer(t)

	header := http.Header{
		"Content-Type":     []string{"application/json"},
		"Content-Encoding": []string{"gzip"},
	}
	resp := do(t, http.MethodPost, ts.URL+"/echo", header, []byte("this is not gzip"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestGzipMiddleware_CompressesBothWays(t *testing.T) {
	ts := newGzipTestServer(t)

	header := http.Header{
		"Content-Type":     []string{"application/json"},
		"Content-Encoding": []string{"gzip"},
		"Accept-Encoding":  []string{"gzip"},
	}
	resp := do(t, http.MethodPost, ts.URL+"/echo", header, mustGzip(t, []byte(jsonBody)))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", enc, "gzip")
	}

	if got := mustGunzip(t, resp.Body); got != jsonBody {
		t.Errorf("decompressed body = %q, want %q", got, jsonBody)
	}
}

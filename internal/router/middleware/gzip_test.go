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

type testResponse struct {
	statusCode    int
	header        http.Header
	contentLength int64
	body          []byte
}

func do(t *testing.T, method, url string, header http.Header, body []byte) testResponse {
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
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	return testResponse{
		statusCode:    resp.StatusCode,
		header:        resp.Header,
		contentLength: resp.ContentLength,
		body:          raw,
	}
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

func mustGunzip(t *testing.T, data []byte) string {
	t.Helper()

	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("response body is not valid gzip: %v", err)
	}
	defer func() { _ = gr.Close() }()

	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to decompress response: %v", err)
	}

	return string(decompressed)
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

			if resp.statusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.statusCode, http.StatusOK)
			}

			enc := resp.header.Get("Content-Encoding")
			if !tt.wantCompress {
				if enc != "" {
					t.Fatalf("Content-Encoding = %q, want empty", enc)
				}

				if string(resp.body) != tt.wantBody {
					t.Errorf("body = %q, want %q", resp.body, tt.wantBody)
				}
				return
			}

			if enc != "gzip" {
				t.Fatalf("Content-Encoding = %q, want %q", enc, "gzip")
			}

			// Content-Length должен описывать сжатое тело. Если бы обёртка не удаляла
			// заголовок, выставленный render.Data, здесь осталась бы длина исходных данных.
			if resp.contentLength != -1 && resp.contentLength != int64(len(resp.body)) {
				t.Errorf("Content-Length = %d, want %d (actual compressed size)", resp.contentLength, len(resp.body))
			}

			if got := mustGunzip(t, resp.body); got != tt.wantBody {
				t.Errorf("decompressed body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestGzipMiddleware_SkipsWithoutAcceptEncoding(t *testing.T) {
	ts := newGzipTestServer(t)

	header := http.Header{"Accept-Encoding": []string{"identity"}}
	resp := do(t, http.MethodGet, ts.URL+"/json", header, nil)

	if resp.statusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.statusCode, http.StatusOK)
	}

	if enc := resp.header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q, want empty", enc)
	}

	if string(resp.body) != jsonBody {
		t.Errorf("body = %q, want %q", resp.body, jsonBody)
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

	if resp.statusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.statusCode, http.StatusOK)
	}

	if string(resp.body) != jsonBody {
		t.Errorf("handler received %q, want %q", resp.body, jsonBody)
	}
}

func TestGzipMiddleware_RejectsInvalidGzipRequest(t *testing.T) {
	ts := newGzipTestServer(t)

	header := http.Header{
		"Content-Type":     []string{"application/json"},
		"Content-Encoding": []string{"gzip"},
	}
	resp := do(t, http.MethodPost, ts.URL+"/echo", header, []byte("this is not gzip"))

	if resp.statusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.statusCode, http.StatusBadRequest)
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

	if resp.statusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.statusCode, http.StatusOK)
	}

	if enc := resp.header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", enc, "gzip")
	}

	if got := mustGunzip(t, resp.body); got != jsonBody {
		t.Errorf("decompressed body = %q, want %q", got, jsonBody)
	}
}

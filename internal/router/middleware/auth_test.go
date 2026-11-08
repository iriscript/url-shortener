package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/iriscript/url-shortener/internal/auth"
	"github.com/iriscript/url-shortener/internal/router/middleware"
)

const authSecret = "test-secret"

func mustToken(t *testing.T, userID string, secret string, ttl time.Duration) string {
	t.Helper()

	token, err := auth.NewToken(userID, []byte(secret), ttl)
	if err != nil {
		t.Fatalf("NewToken failed: %v", err)
	}

	return token
}

// newAuthTestServer answers with the resolved user id and whether it was authenticated, so that
// tests can assert on what the middleware put into the request context.
func newAuthTestServer(t *testing.T, extra ...gin.HandlerFunc) *httptest.Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.AuthMiddleware(authSecret))
	for _, h := range extra {
		router.Use(h)
	}
	router.GET("/whoami", func(c *gin.Context) {
		userID, authenticated := middleware.UserID(c)

		c.Header("X-Authenticated", map[bool]string{true: "true", false: "false"}[authenticated])
		c.Data(http.StatusOK, "text/plain", []byte(userID))
	})

	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return ts
}

func whoami(t *testing.T, ts *httptest.Server, cookie *http.Cookie) (userID string, authenticated bool, issued *http.Cookie) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/whoami", nil)
	if err != nil {
		t.Fatalf("failed to build the request: %v", err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read the response body: %v", err)
	}

	for _, c := range resp.Cookies() {
		if c.Name == "auth" {
			issued = c
		}
	}

	return string(body), resp.Header.Get("X-Authenticated") == "true", issued
}

func TestAuthMiddleware_IssuesCookieWhenAbsent(t *testing.T) {
	ts := newAuthTestServer(t)

	userID, authenticated, issued := whoami(t, ts, nil)

	if userID == "" {
		t.Error("middleware did not put a user id into the context")
	}
	if !authenticated {
		t.Error("a request without a cookie was reported as unauthenticated, want authenticated")
	}
	if issued == nil {
		t.Fatal("middleware did not issue the auth cookie")
	}

	signedID, err := auth.ParseToken(issued.Value, []byte(authSecret))
	if err != nil {
		t.Fatalf("the issued cookie does not pass its own check: %v", err)
	}
	if signedID != userID {
		t.Errorf("cookie carries user id %q, want the context one %q", signedID, userID)
	}
	if !issued.HttpOnly || issued.Path != "/" {
		t.Errorf("cookie = %+v, want HttpOnly on path /", issued)
	}
}

// TestAuthMiddleware_IgnoresForeignStringKeys pins the reason the context keys carry an unexported
// type: a package that happens to use the same plain string names cannot reach the middleware's
// entries, so it cannot overwrite them.
func TestAuthMiddleware_IgnoresForeignStringKeys(t *testing.T) {
	foreign := func(c *gin.Context) {
		c.Set("userID", "someoneElse")
		c.Set("authenticated", false)
		c.Next()
	}

	ts := newAuthTestServer(t, foreign)

	userID, authenticated, issued := whoami(t, ts, nil)

	if issued == nil {
		t.Fatal("middleware did not issue the auth cookie")
	}
	if userID == "someoneElse" {
		t.Error("a foreign string key overwrote the user id in the context")
	}
	if !authenticated {
		t.Error("a foreign string key overwrote the authentication flag in the context")
	}

	want, err := auth.ParseToken(issued.Value, []byte(authSecret))
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if userID != want {
		t.Errorf("user id = %q, want the one from the cookie %q", userID, want)
	}
}

func TestAuthMiddleware_ReusesAuthenticCookie(t *testing.T) {
	ts := newAuthTestServer(t)

	_, _, issued := whoami(t, ts, nil)
	if issued == nil {
		t.Fatal("middleware did not issue the auth cookie")
	}

	userID, authenticated, reissued := whoami(t, ts, issued)

	if !authenticated {
		t.Error("an authentic cookie was reported as unauthenticated")
	}
	if reissued != nil {
		t.Errorf("middleware re-issued the cookie %+v, want the existing one kept", reissued)
	}

	want, err := auth.ParseToken(issued.Value, []byte(authSecret))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if userID != want {
		t.Errorf("user id = %q, want %q", userID, want)
	}
}

func TestAuthMiddleware_ReissuesUntrustedCookie(t *testing.T) {
	ts := newAuthTestServer(t)

	tests := []struct {
		name  string
		value string
	}{
		{name: "not a token", value: "someUserID"},
		{name: "signed with another secret", value: mustToken(t, "someUserID", "another-secret", time.Hour)},
		{name: "expired", value: mustToken(t, "someUserID", authSecret, -time.Hour)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, authenticated, issued := whoami(t, ts, &http.Cookie{Name: "auth", Value: tt.value})

			if authenticated {
				t.Error("a cookie that is not authentic was reported as authenticated")
			}
			if userID == "" {
				t.Error("middleware did not put a fresh user id into the context")
			}
			if issued == nil {
				t.Fatal("middleware did not re-issue the auth cookie")
			}
			if _, err := auth.ParseToken(issued.Value, []byte(authSecret)); err != nil {
				t.Errorf("the re-issued cookie does not pass its own check: %v", err)
			}
		})
	}
}

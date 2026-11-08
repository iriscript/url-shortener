package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/iriscript/url-shortener/internal/auth"
)

var secret = []byte("test-secret")

const testTTL = time.Hour

func mustToken(t *testing.T, userID string, secret []byte, ttl time.Duration) string {
	t.Helper()

	token, err := auth.NewToken(userID, secret, ttl)
	if err != nil {
		t.Fatalf("NewToken failed: %v", err)
	}

	return token
}

func signClaims(t *testing.T, method jwt.SigningMethod, key any, claims jwt.Claims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign the test token: %v", err)
	}

	return token
}

func decodeSegment(t *testing.T, token string, index int) map[string]any {
	t.Helper()

	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		t.Fatalf("token has %d segments, want 3", len(segments))
	}

	raw, err := base64.RawURLEncoding.DecodeString(segments[index])
	if err != nil {
		t.Fatalf("segment %d is not base64url: %v", index, err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("segment %d is not JSON: %v", index, err)
	}

	return decoded
}

func TestNewTokenParseToken_RoundTrip(t *testing.T) {
	userID, err := auth.NewUserID()
	if err != nil {
		t.Fatalf("NewUserID failed: %v", err)
	}

	got, err := auth.ParseToken(mustToken(t, userID, secret, testTTL), secret)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if got != userID {
		t.Errorf("ParseToken = %q, want %q", got, userID)
	}
}

func TestNewToken_IsSelfDescribing(t *testing.T) {
	issuedAt := time.Now()
	token := mustToken(t, "someUserID", secret, testTTL)

	header := decodeSegment(t, token, 0)
	if header["alg"] != jwt.SigningMethodHS256.Alg() {
		t.Errorf("header alg = %v, want %v", header["alg"], jwt.SigningMethodHS256.Alg())
	}
	if header["typ"] != "JWT" {
		t.Errorf("header typ = %v, want JWT", header["typ"])
	}

	claims := decodeSegment(t, token, 1)
	if claims["sub"] != "someUserID" {
		t.Errorf("sub = %v, want someUserID", claims["sub"])
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatalf("exp = %v, want a number", claims["exp"])
	}
	if _, ok := claims["iat"].(float64); !ok {
		t.Fatalf("iat = %v, want a number", claims["iat"])
	}

	// The expiry is a second-precision timestamp, so compare with a second of slack either way.
	wantExp := issuedAt.Add(testTTL).Unix()
	if delta := int64(exp) - wantExp; delta < -1 || delta > 1 {
		t.Errorf("exp = %d, want about %d", int64(exp), wantExp)
	}
}

func TestParseToken_RejectsInvalidValues(t *testing.T) {
	valid := mustToken(t, "someUserID", secret, testTTL)
	segments := strings.Split(valid, ".")

	tamperedClaims := make([]string, len(segments))
	copy(tamperedClaims, segments)
	tamperedClaims[1] = "f" + tamperedClaims[1][1:]

	tests := []struct {
		name   string
		value  string
		secret []byte
	}{
		{
			name:   "empty value",
			value:  "",
			secret: secret,
		},
		{
			name:   "not a token",
			value:  "someUserID",
			secret: secret,
		},
		{
			name:   "signature segment is missing",
			value:  strings.Join(segments[:2], "."),
			secret: secret,
		},
		{
			name:   "signature is not base64url",
			value:  strings.Join(segments[:2], ".") + ".notabase64signature!",
			secret: secret,
		},
		{
			name:   "tampered claims",
			value:  strings.Join(tamperedClaims, "."),
			secret: secret,
		},
		{
			name:   "signed with another secret",
			value:  mustToken(t, "someUserID", []byte("another-secret"), testTTL),
			secret: secret,
		},
		{
			name: "signed with another algorithm",
			value: signClaims(t, jwt.SigningMethodHS512, secret, jwt.RegisteredClaims{
				Subject:   "someUserID",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(testTTL)),
			}),
			secret: secret,
		},
		{
			name: "not signed at all",
			value: signClaims(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, jwt.RegisteredClaims{
				Subject:   "someUserID",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(testTTL)),
			}),
			secret: secret,
		},
		{
			name:   "no expiry",
			value:  signClaims(t, jwt.SigningMethodHS256, secret, jwt.RegisteredClaims{Subject: "someUserID"}),
			secret: secret,
		},
		{
			name:   "expired",
			value:  mustToken(t, "someUserID", secret, -time.Hour),
			secret: secret,
		},
		{
			name:   "empty subject",
			value:  mustToken(t, "", secret, testTTL),
			secret: secret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, err := auth.ParseToken(tt.value, tt.secret)
			if !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("ParseToken error = %v, want %v", err, auth.ErrInvalidToken)
			}
			if userID != "" {
				t.Errorf("ParseToken returned user id %q, want an empty string", userID)
			}
		})
	}
}

func TestNewUserID_ReturnsDistinctValues(t *testing.T) {
	const iterations = 100

	seen := make(map[string]struct{}, iterations)
	for range iterations {
		userID, err := auth.NewUserID()
		if err != nil {
			t.Fatalf("NewUserID failed: %v", err)
		}
		if userID == "" {
			t.Fatal("NewUserID returned an empty id")
		}
		if _, duplicate := seen[userID]; duplicate {
			t.Fatalf("NewUserID returned a duplicate id %q", userID)
		}

		seen[userID] = struct{}{}
	}
}

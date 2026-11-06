package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/iriscript/url-shortener/internal/auth"
)

var secret = []byte("test-secret")

func TestSignParse_RoundTrip(t *testing.T) {
	userID, err := auth.NewUserID()
	if err != nil {
		t.Fatalf("NewUserID failed: %v", err)
	}

	got, err := auth.Parse(auth.Sign(userID, secret), secret)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got != userID {
		t.Errorf("Parse = %q, want %q", got, userID)
	}
}

func TestParse_RejectsInvalidValues(t *testing.T) {
	signed := auth.Sign("someUserID", secret)

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
			name:   "no separator",
			value:  "someUserID",
			secret: secret,
		},
		{
			name:   "empty user id",
			value:  auth.Sign("", secret),
			secret: secret,
		},
		{
			name:   "signature is not hex",
			value:  "someUserID.notahexsignature",
			secret: secret,
		},
		{
			name:   "truncated signature",
			value:  signed[:len(signed)-2],
			secret: secret,
		},
		{
			name:   "tampered user id",
			value:  "otherUserID" + signed[strings.Index(signed, "."):],
			secret: secret,
		},
		{
			name:   "signed with another secret",
			value:  auth.Sign("someUserID", []byte("another-secret")),
			secret: secret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, err := auth.Parse(tt.value, tt.secret)
			if !errors.Is(err, auth.ErrInvalidCookie) {
				t.Fatalf("Parse error = %v, want %v", err, auth.ErrInvalidCookie)
			}
			if userID != "" {
				t.Errorf("Parse returned user id %q, want an empty string", userID)
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

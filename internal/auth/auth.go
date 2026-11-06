package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	userIDSize = 16
	separator  = "."
)

var ErrInvalidCookie = errors.New("cookie is not authentic")

func NewUserID() (string, error) {
	b := make([]byte, userIDSize)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate the user id: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func Sign(userID string, secret []byte) string {
	return userID + separator + hex.EncodeToString(sign(userID, secret))
}

func Parse(value string, secret []byte) (string, error) {
	userID, signature, found := strings.Cut(value, separator)
	if !found || userID == "" {
		return "", ErrInvalidCookie
	}

	got, err := hex.DecodeString(signature)
	if err != nil {
		return "", ErrInvalidCookie
	}
	if !hmac.Equal(got, sign(userID, secret)) {
		return "", ErrInvalidCookie
	}

	return userID, nil
}

func sign(userID string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(userID))

	return mac.Sum(nil)
}

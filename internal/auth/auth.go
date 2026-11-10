package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const userIDSize = 16

var signingMethod = jwt.SigningMethodHS256

var ErrInvalidToken = errors.New("token is not authentic")

func NewUserID() (string, error) {
	b := make([]byte, userIDSize)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate the user id: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func NewToken(userID string, secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()

	token := jwt.NewWithClaims(signingMethod, jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	})

	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign the token: %w", err)
	}

	return signed, nil
}

func ParseToken(value string, secret []byte) (string, error) {
	var claims jwt.RegisteredClaims

	_, err := jwt.ParseWithClaims(value, &claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", ErrInvalidToken
	}
	if claims.Subject == "" {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}

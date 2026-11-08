package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/iriscript/url-shortener/internal/auth"
	"github.com/rs/zerolog/log"
)

const (
	authCookieName = "auth"
	authCookieTTL  = 30 * 24 * time.Hour
)

type contextKey string

const (
	contextUserID        contextKey = "userID"
	contextAuthenticated contextKey = "authenticated"
)

func AuthMiddleware(secret string) gin.HandlerFunc {
	key := []byte(secret)

	return func(c *gin.Context) {
		userID, authenticated, err := resolveUser(c, key)
		if err != nil {
			log.Error().Err(err).Msg("auth: failed to issue the user id")
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		c.Set(contextUserID, userID)
		c.Set(contextAuthenticated, authenticated)

		c.Next()
	}
}

func UserID(c *gin.Context) (string, bool) {
	userID := c.GetString(contextUserID)

	return userID, userID != "" && c.GetBool(contextAuthenticated)
}

func resolveUser(c *gin.Context, key []byte) (string, bool, error) {
	cookie, err := c.Request.Cookie(authCookieName)
	if err == nil {
		userID, parseErr := auth.ParseToken(cookie.Value, key)
		if parseErr == nil {
			return userID, true, nil
		}
		if !errors.Is(parseErr, auth.ErrInvalidToken) {
			return "", false, parseErr
		}

		log.Warn().Msg("auth: rejected a cookie that is not authentic")

		userID, err := issue(c, key)
		if err != nil {
			return "", false, err
		}

		return userID, false, nil
	}

	userID, err := issue(c, key)
	if err != nil {
		return "", false, err
	}

	return userID, true, nil
}

func issue(c *gin.Context, key []byte) (string, error) {
	userID, err := auth.NewUserID()
	if err != nil {
		return "", err
	}

	token, err := auth.NewToken(userID, key, authCookieTTL)
	if err != nil {
		return "", err
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(authCookieTTL.Seconds()),
		HttpOnly: true,
	})

	return userID, nil
}

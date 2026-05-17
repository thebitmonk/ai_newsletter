package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/httpx"
)

const (
	ctxSessionKey = "auth.session"
)

// Bearer validates the Authorization: Bearer <token> header against the
// session store and stashes the resolved Session in the request context.
// Requests with missing/malformed/expired tokens are rejected with 401.
func Bearer(sessions *SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			httpx.Error(c, http.StatusUnauthorized, "missing_token", "Authorization header is required")
			return
		}
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			httpx.Error(c, http.StatusUnauthorized, "malformed_token", "Authorization header must use Bearer scheme")
			return
		}
		raw := strings.TrimSpace(header[len(prefix):])
		if raw == "" {
			httpx.Error(c, http.StatusUnauthorized, "missing_token", "Bearer token is empty")
			return
		}

		sess, err := sessions.Lookup(c.Request.Context(), raw)
		if err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				httpx.Error(c, http.StatusUnauthorized, "invalid_token", "session not found or expired")
				return
			}
			httpx.Error(c, http.StatusInternalServerError, "internal", "session lookup failed")
			return
		}

		c.Set(ctxSessionKey, sess)
		c.Next()
	}
}

// RequireAccountScope refuses requests that lack a Session in context (i.e.
// the Bearer middleware did not run or did not set one). Handlers can then
// read AccountID(c) / UserID(c) safely.
func RequireAccountScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get(ctxSessionKey); !ok {
			httpx.Error(c, http.StatusUnauthorized, "missing_scope", "account scope is required")
			return
		}
		c.Next()
	}
}

// AccountID returns the authenticated account ID for the current request.
// Panics if no Session is set — callers must be behind Bearer + RequireAccountScope.
func AccountID(c *gin.Context) uuid.UUID {
	return sessionFrom(c).AccountID
}

// UserID returns the authenticated user ID for the current request.
// Panics if no Session is set — callers must be behind Bearer + RequireAccountScope.
func UserID(c *gin.Context) uuid.UUID {
	return sessionFrom(c).UserID
}

func sessionFrom(c *gin.Context) *Session {
	v, ok := c.Get(ctxSessionKey)
	if !ok {
		panic("auth: no session in context — handler is not behind Bearer middleware")
	}
	return v.(*Session)
}

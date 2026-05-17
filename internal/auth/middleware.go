// Package auth provides the HTTP middleware that turns a Firebase ID token
// (Authorization: Bearer <token>) into a request-scoped Session with userID +
// accountID. Per ADR-0016 the verification is delegated to firebaseauth; the
// upsert to firebaseauth.Claims → (User, Account) is delegated to users.
package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/firebaseauth"
	"github.com/thebitmonk/ai_newsletter/internal/httpx"
	"github.com/thebitmonk/ai_newsletter/internal/users"
)

const ctxSessionKey = "auth.session"

// Session is the per-request authentication context downstream handlers read
// via UserID / AccountID accessors.
type Session struct {
	UserID    uuid.UUID
	AccountID uuid.UUID
	Claims    *firebaseauth.Claims
}

// Bearer validates Authorization: Bearer <firebase-id-token>, upserts the
// User + Account (first request from a new UID atomically creates both per
// ADR-0013 / ADR-0016), and stashes the Session in the request context.
func Bearer(verifier firebaseauth.TokenVerifier, store *users.Store) gin.HandlerFunc {
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

		claims, err := verifier.Verify(c.Request.Context(), raw)
		if err != nil {
			if errors.Is(err, firebaseauth.ErrInvalidToken) {
				httpx.Error(c, http.StatusUnauthorized, "invalid_token", "id token is invalid or expired")
				return
			}
			httpx.Error(c, http.StatusInternalServerError, "internal", "token verification failed")
			return
		}

		userID, accountID, err := store.GetOrCreateByFirebaseUID(c.Request.Context(), claims)
		if err != nil {
			httpx.Error(c, http.StatusInternalServerError, "internal", "user upsert failed")
			return
		}

		c.Set(ctxSessionKey, &Session{
			UserID:    userID,
			AccountID: accountID,
			Claims:    claims,
		})
		c.Next()
	}
}

// RequireAccountScope refuses requests that lack a Session in context (i.e.
// Bearer did not run or did not set one).
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
func AccountID(c *gin.Context) uuid.UUID { return sessionFrom(c).AccountID }

// UserID returns the authenticated user ID for the current request.
func UserID(c *gin.Context) uuid.UUID { return sessionFrom(c).UserID }

// CurrentClaims exposes the verified Firebase claims for handlers that need
// e.g. the user's email for display.
func CurrentClaims(c *gin.Context) *firebaseauth.Claims { return sessionFrom(c).Claims }

func sessionFrom(c *gin.Context) *Session {
	v, ok := c.Get(ctxSessionKey)
	if !ok {
		panic("auth: no session in context — handler is not behind Bearer middleware")
	}
	return v.(*Session)
}

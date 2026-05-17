package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/auth"
	"github.com/thebitmonk/ai_newsletter/internal/publications"
)

// New returns a fully-wired Gin engine. The caller owns the pool's lifecycle.
func New(pool *pgxpool.Pool) *gin.Engine {
	sessions := auth.NewSessionStore(pool)
	authHandlers := auth.NewHandlers(pool, sessions)
	pubHandlers := publications.NewHandlers(publications.NewStore(pool))

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", healthz(pool))

	v1 := r.Group("/api/v1")
	authHandlers.Register(v1)

	authed := v1.Group("/", auth.Bearer(sessions), auth.RequireAccountScope())
	authed.GET("/whoami", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":    auth.UserID(c),
			"account_id": auth.AccountID(c),
		})
	})
	pubHandlers.Register(authed)

	return r
}

func healthz(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "db": "down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "ok"})
	}
}

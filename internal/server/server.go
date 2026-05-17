package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/auth"
	"github.com/thebitmonk/ai_newsletter/internal/publications"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

type config struct {
	sourcePostCreate sources.PostCreateHook
}

// Option mutates the server config.
type Option func(*config)

// WithSourcePostCreateHook wires a callback fired after each successful
// Source create. The poller uses this to bootstrap polling for a newly added
// Source immediately rather than waiting for the supervisor sweep.
func WithSourcePostCreateHook(h sources.PostCreateHook) Option {
	return func(c *config) { c.sourcePostCreate = h }
}

// New returns a fully-wired Gin engine. The caller owns the pool's lifecycle.
func New(pool *pgxpool.Pool, opts ...Option) *gin.Engine {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	sessions := auth.NewSessionStore(pool)
	authHandlers := auth.NewHandlers(pool, sessions)
	pubHandlers := publications.NewHandlers(publications.NewStore(pool))
	srcHandlers := sources.NewHandlers(sources.NewStore(pool))
	if cfg.sourcePostCreate != nil {
		srcHandlers.SetPostCreateHook(cfg.sourcePostCreate)
	}

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
	srcHandlers.Register(authed)

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

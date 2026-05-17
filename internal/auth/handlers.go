package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/httpx"
)

const minPasswordLen = 8

type Handlers struct {
	pool     *pgxpool.Pool
	sessions *SessionStore
}

func NewHandlers(pool *pgxpool.Pool, sessions *SessionStore) *Handlers {
	return &Handlers{pool: pool, sessions: sessions}
}

func (h *Handlers) Register(r gin.IRouter) {
	g := r.Group("/auth")
	g.POST("/signup", h.signup)
	g.POST("/login", h.login)
}

type credentialsReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type authResp struct {
	Token     string    `json:"token"`
	UserID    uuid.UUID `json:"user_id"`
	AccountID uuid.UUID `json:"account_id"`
}

func (h *Handlers) signup(c *gin.Context) {
	var req credentialsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if len(req.Password) < minPasswordLen {
		httpx.Error(c, http.StatusBadRequest, "password_too_short",
			"password must be at least 8 characters")
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", "could not hash password")
		return
	}

	var token string
	var userID, accountID uuid.UUID

	err = withTx(c.Request.Context(), h.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(c.Request.Context(),
			`insert into users (email, password_hash) values ($1, $2) returning id`,
			req.Email, hash,
		).Scan(&userID); err != nil {
			return err
		}
		if err := tx.QueryRow(c.Request.Context(),
			`insert into accounts default values returning id`,
		).Scan(&accountID); err != nil {
			return err
		}
		if _, err := tx.Exec(c.Request.Context(),
			`insert into account_members (account_id, user_id, role) values ($1, $2, 'owner')`,
			accountID, userID,
		); err != nil {
			return err
		}
		t, err := h.sessions.Create(c.Request.Context(), tx, userID, accountID)
		if err != nil {
			return err
		}
		token = t
		return nil
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			httpx.Error(c, http.StatusConflict, "email_taken", "email is already registered")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "internal", "signup failed")
		return
	}

	c.JSON(http.StatusCreated, authResp{Token: token, UserID: userID, AccountID: accountID})
}

func (h *Handlers) login(c *gin.Context) {
	var req credentialsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	var (
		userID    uuid.UUID
		accountID uuid.UUID
		hash      string
	)
	err := h.pool.QueryRow(c.Request.Context(), `
		select u.id, u.password_hash, am.account_id
		from users u
		join account_members am on am.user_id = u.id
		where u.email = $1
	`, req.Email).Scan(&userID, &hash, &accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(c, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
			return
		}
		httpx.Error(c, http.StatusInternalServerError, "internal", "login failed")
		return
	}

	if !CheckPassword(req.Password, hash) {
		httpx.Error(c, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		return
	}

	token, err := h.sessions.Create(c.Request.Context(), h.pool, userID, accountID)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", "could not issue session")
		return
	}

	c.JSON(http.StatusOK, authResp{Token: token, UserID: userID, AccountID: accountID})
}

func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

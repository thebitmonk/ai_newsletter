package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	tokenBytes = 32
	SessionTTL = 30 * 24 * time.Hour
)

var ErrSessionNotFound = errors.New("session not found or expired")

type Session struct {
	UserID    uuid.UUID
	AccountID uuid.UUID
	ExpiresAt time.Time
}

// Querier is the subset of pgx methods used by SessionStore so callers can
// pass either a pool or a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type SessionStore struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

func generateToken() (raw string, hash []byte, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256(b)
	return raw, sum[:], nil
}

func hashToken(raw string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

// Create issues a session for the given user/account, returning the raw token
// the client should present in subsequent requests. Pass either the pool or a
// transaction as q.
func (s *SessionStore) Create(ctx context.Context, q Querier, userID, accountID uuid.UUID) (string, error) {
	raw, hash, err := generateToken()
	if err != nil {
		return "", err
	}
	if _, err := q.Exec(ctx,
		`insert into sessions (token_hash, user_id, account_id, expires_at) values ($1, $2, $3, $4)`,
		hash, userID, accountID, time.Now().Add(SessionTTL),
	); err != nil {
		return "", err
	}
	return raw, nil
}

// Lookup resolves a raw token to a Session, returning ErrSessionNotFound if
// the token is unknown or expired.
func (s *SessionStore) Lookup(ctx context.Context, rawToken string) (*Session, error) {
	hash, err := hashToken(rawToken)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	var sess Session
	err = s.pool.QueryRow(ctx,
		`select user_id, account_id, expires_at from sessions where token_hash = $1 and expires_at > now()`,
		hash,
	).Scan(&sess.UserID, &sess.AccountID, &sess.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &sess, nil
}

// Revoke deletes a session by raw token. Idempotent — returns nil if the
// token did not exist.
func (s *SessionStore) Revoke(ctx context.Context, rawToken string) error {
	hash, err := hashToken(rawToken)
	if err != nil {
		return nil
	}
	_, err = s.pool.Exec(ctx, `delete from sessions where token_hash = $1`, hash)
	return err
}

// Package users persists User + Account identity rows backed by Firebase
// per ADR-0016. The hot path is GetOrCreateByFirebaseUID, called once per
// authed request by the Bearer middleware. It must converge concurrent
// first-requests for the same UID onto one Account — the unique constraint
// on users.firebase_uid is what makes that safe.
package users

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/firebaseauth"
)

type User struct {
	ID            uuid.UUID
	FirebaseUID   string
	Email         string
	EmailVerified bool
	CreatedAt     time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// GetOrCreateByFirebaseUID resolves a Firebase UID to a (User, Account) pair.
// On first request for a UID, atomically creates User + Account + account_members
// in one transaction. On subsequent requests, returns the existing pair and
// updates cached email + email_verified from claims (cheap UPDATE no-op when
// unchanged).
//
// Concurrent first-requests for the same UID race on the unique index on
// firebase_uid: one INSERT wins, the loser sees 23505 and re-reads the row.
// The result is the same Account either way.
func (s *Store) GetOrCreateByFirebaseUID(ctx context.Context, claims *firebaseauth.Claims) (userID, accountID uuid.UUID, err error) {
	if claims.UID == "" {
		return uuid.Nil, uuid.Nil, errors.New("users: empty firebase_uid in claims")
	}

	// Fast path: existing user.
	uid, aid, found, err := s.lookup(ctx, claims)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if found {
		return uid, aid, nil
	}

	// Slow path: create.
	uid, aid, err = s.createAtomic(ctx, claims)
	if err == nil {
		return uid, aid, nil
	}

	// Race: another goroutine inserted between our lookup and our insert.
	// Re-read and return that row.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		uid, aid, found, lerr := s.lookup(ctx, claims)
		if lerr != nil {
			return uuid.Nil, uuid.Nil, lerr
		}
		if !found {
			return uuid.Nil, uuid.Nil, errors.New("users: post-conflict lookup found no row")
		}
		return uid, aid, nil
	}
	return uuid.Nil, uuid.Nil, err
}

// lookup returns the (user_id, account_id) for an existing UID and also keeps
// cached email + email_verified up to date with the claims.
func (s *Store) lookup(ctx context.Context, claims *firebaseauth.Claims) (userID, accountID uuid.UUID, found bool, err error) {
	row := s.pool.QueryRow(ctx, `
		select u.id, am.account_id
		from users u
		join account_members am on am.user_id = u.id
		where u.firebase_uid = $1
	`, claims.UID)

	if err := row.Scan(&userID, &accountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, false, nil
		}
		return uuid.Nil, uuid.Nil, false, err
	}

	// Refresh cached email + email_verified. The UPDATE is conditional so we
	// don't spam updated_at-touching writes for unchanged values.
	_, err = s.pool.Exec(ctx, `
		update users
		set email          = coalesce(nullif($2, ''), email),
		    email_verified = $3
		where id = $1
		  and (coalesce(email, '') is distinct from coalesce($2, email)
		       or email_verified is distinct from $3)
	`, userID, claims.Email, claims.EmailVerified)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, err
	}
	return userID, accountID, true, nil
}

// createAtomic inserts User + Account + account_members in one transaction.
func (s *Store) createAtomic(ctx context.Context, claims *firebaseauth.Claims) (userID, accountID uuid.UUID, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx, `
		insert into users (firebase_uid, email, email_verified)
		values ($1, nullif($2, ''), $3)
		returning id
	`, claims.UID, claims.Email, claims.EmailVerified).Scan(&userID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if err := tx.QueryRow(ctx, `insert into accounts default values returning id`).Scan(&accountID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if _, err := tx.Exec(ctx,
		`insert into account_members (account_id, user_id, role) values ($1, $2, 'owner')`,
		accountID, userID,
	); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return userID, accountID, nil
}

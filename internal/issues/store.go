package issues

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("issue not found")

type Issue struct {
	ID            uuid.UUID
	PublicationID uuid.UUID
	State         State
	ScheduledAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreatePlanned inserts a new `planned` Issue. If an Issue with the same
// (publication_id, scheduled_at) already exists, returns the existing Issue
// and created=false. This is the idempotency contract the scheduler relies on.
func (s *Store) CreatePlanned(ctx context.Context, publicationID uuid.UUID, scheduledAt time.Time) (*Issue, bool, error) {
	row := s.pool.QueryRow(ctx, `
		insert into issues (publication_id, state, scheduled_at)
		values ($1, 'planned', $2)
		on conflict (publication_id, scheduled_at) do nothing
		returning id, publication_id, state, scheduled_at, created_at, updated_at
	`, publicationID, scheduledAt)

	iss, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict path — fetch the existing row.
		existing, gerr := s.GetByScheduledAt(ctx, publicationID, scheduledAt)
		if gerr != nil {
			return nil, false, gerr
		}
		return existing, false, nil
	}
	if err != nil {
		// Surface FK violation distinctly so the scheduler can skip cleanly.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	return iss, true, nil
}

func (s *Store) GetByScheduledAt(ctx context.Context, publicationID uuid.UUID, scheduledAt time.Time) (*Issue, error) {
	row := s.pool.QueryRow(ctx, `
		select id, publication_id, state, scheduled_at, created_at, updated_at
		from issues
		where publication_id = $1 and scheduled_at = $2
	`, publicationID, scheduledAt)
	iss, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return iss, err
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (*Issue, error) {
	row := s.pool.QueryRow(ctx, `
		select id, publication_id, state, scheduled_at, created_at, updated_at
		from issues
		where id = $1
	`, id)
	iss, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return iss, err
}

// ListByPublication returns all Issues for a Publication, newest scheduled
// first. Used by tests and the future calendar API.
func (s *Store) ListByPublication(ctx context.Context, publicationID uuid.UUID) ([]Issue, error) {
	rows, err := s.pool.Query(ctx, `
		select id, publication_id, state, scheduled_at, created_at, updated_at
		from issues
		where publication_id = $1
		order by scheduled_at asc
	`, publicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Issue
	for rows.Next() {
		var iss Issue
		if err := rows.Scan(&iss.ID, &iss.PublicationID, &iss.State, &iss.ScheduledAt, &iss.CreatedAt, &iss.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, iss)
	}
	return out, rows.Err()
}

func scanIssue(row pgx.Row) (*Issue, error) {
	var iss Issue
	if err := row.Scan(&iss.ID, &iss.PublicationID, &iss.State, &iss.ScheduledAt, &iss.CreatedAt, &iss.UpdatedAt); err != nil {
		return nil, err
	}
	return &iss, nil
}

package issues

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("issue not found")

type Issue struct {
	ID             uuid.UUID
	PublicationID  uuid.UUID
	State          State
	ScheduledAt    time.Time
	Subject        *string
	Title          *string
	CoverURL       *string
	BodyDoc        json.RawMessage // null when planned, populated when drafted+
	BodyDocVersion int
	FailedReason   *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
		returning `+issueColumns+`
	`, publicationID, scheduledAt)

	iss, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gerr := s.GetByScheduledAt(ctx, publicationID, scheduledAt)
		if gerr != nil {
			return nil, false, gerr
		}
		return existing, false, nil
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	return iss, true, nil
}

func (s *Store) GetByScheduledAt(ctx context.Context, publicationID uuid.UUID, scheduledAt time.Time) (*Issue, error) {
	row := s.pool.QueryRow(ctx,
		`select `+issueColumns+` from issues where publication_id = $1 and scheduled_at = $2`,
		publicationID, scheduledAt)
	iss, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return iss, err
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (*Issue, error) {
	row := s.pool.QueryRow(ctx,
		`select `+issueColumns+` from issues where id = $1`, id)
	iss, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return iss, err
}

// GetForAccount returns the Issue if its parent Publication belongs to
// accountID, else ErrNotFound. Used by HTTP handlers.
func (s *Store) GetForAccount(ctx context.Context, accountID, id uuid.UUID) (*Issue, error) {
	row := s.pool.QueryRow(ctx, `
		select `+prefixedIssueColumns("i")+`
		from issues i
		join publications p on p.id = i.publication_id
		where i.id = $1 and p.account_id = $2
	`, id, accountID)
	iss, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return iss, err
}

// ListByPublication returns all Issues for a Publication, scheduled-ascending.
func (s *Store) ListByPublication(ctx context.Context, publicationID uuid.UUID) ([]Issue, error) {
	rows, err := s.pool.Query(ctx,
		`select `+issueColumns+` from issues where publication_id = $1 order by scheduled_at asc`,
		publicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Issue
	for rows.Next() {
		iss, err := scanIssueRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *iss)
	}
	return out, rows.Err()
}

// ApplyTransition loads the Issue, validates the (state, event) transition,
// updates state + the supplied content fields (any non-nil), persists. Use
// this for every state-changing write — no direct UPDATE state=... allowed
// elsewhere.
type TransitionUpdate struct {
	Subject      *string
	Title        *string
	CoverURL     *string
	BodyDoc      json.RawMessage
	FailedReason *string
}

func (s *Store) ApplyTransition(ctx context.Context, id uuid.UUID, event Event, upd TransitionUpdate) (*Issue, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx,
		`select `+issueColumns+` from issues where id = $1 for update`, id)
	current, err := scanIssue(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	next, err := Transition(current.State, event)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		update issues set
		    state         = $2,
		    subject       = coalesce($3, subject),
		    title         = coalesce($4, title),
		    cover_url     = coalesce($5, cover_url),
		    body_doc      = case when $6::jsonb is null then body_doc else $6::jsonb end,
		    failed_reason = $7,
		    updated_at    = now()
		where id = $1
	`, id, next, upd.Subject, upd.Title, upd.CoverURL, upd.BodyDoc, upd.FailedReason)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

const issueColumns = `id, publication_id, state, scheduled_at,
		    subject, title, cover_url, body_doc, body_doc_version,
		    failed_reason, created_at, updated_at`

func prefixedIssueColumns(alias string) string {
	cols := []string{"id", "publication_id", "state", "scheduled_at",
		"subject", "title", "cover_url", "body_doc", "body_doc_version",
		"failed_reason", "created_at", "updated_at"}
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ", "
		}
		out += alias + "." + c
	}
	return out
}

func scanIssue(row pgx.Row) (*Issue, error) {
	var iss Issue
	if err := row.Scan(
		&iss.ID, &iss.PublicationID, &iss.State, &iss.ScheduledAt,
		&iss.Subject, &iss.Title, &iss.CoverURL, &iss.BodyDoc, &iss.BodyDocVersion,
		&iss.FailedReason, &iss.CreatedAt, &iss.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &iss, nil
}

func scanIssueRows(rows pgx.Rows) (*Issue, error) {
	var iss Issue
	if err := rows.Scan(
		&iss.ID, &iss.PublicationID, &iss.State, &iss.ScheduledAt,
		&iss.Subject, &iss.Title, &iss.CoverURL, &iss.BodyDoc, &iss.BodyDocVersion,
		&iss.FailedReason, &iss.CreatedAt, &iss.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &iss, nil
}

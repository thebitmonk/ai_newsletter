package sources

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("source not found")
	ErrDuplicate     = errors.New("source with this type+identifier already exists for this publication")
	ErrPublicationNF = errors.New("parent publication not found")
)

type Type string

const (
	TypeRSS            Type = "rss"
	TypeYouTubeChannel Type = "youtube_channel"
	TypeXHandle        Type = "x_handle"
	TypeSubstack       Type = "substack"
	TypeWeb            Type = "web"
)

// DefaultPollInterval returns the per-type recommended poll cadence.
func DefaultPollInterval(t Type) time.Duration {
	switch t {
	case TypeRSS, TypeSubstack:
		return time.Hour
	case TypeYouTubeChannel:
		return 6 * time.Hour
	case TypeXHandle:
		return 30 * time.Minute
	case TypeWeb:
		return 4 * time.Hour
	}
	return time.Hour
}

func IsValidType(s string) bool {
	switch Type(s) {
	case TypeRSS, TypeYouTubeChannel, TypeXHandle, TypeSubstack, TypeWeb:
		return true
	}
	return false
}

type Source struct {
	ID            uuid.UUID     `json:"id"`
	PublicationID uuid.UUID     `json:"publication_id"`
	Type          Type          `json:"type"`
	Identifier    string        `json:"identifier"`
	PollInterval  time.Duration `json:"poll_interval_ns"`
	Enabled       bool          `json:"enabled"`
	LastPolledAt  *time.Time    `json:"last_polled_at,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type CreateParams struct {
	Type         Type
	Identifier   string
	PollInterval *time.Duration
	Enabled      *bool
}

type UpdateParams struct {
	Identifier   *string
	PollInterval *time.Duration
	Enabled      *bool
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// publicationBelongsToAccount returns true if the publication exists and is
// owned by the given account.
func (s *Store) publicationBelongsToAccount(ctx context.Context, accountID, publicationID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`select exists(select 1 from publications where id = $1 and account_id = $2)`,
		publicationID, accountID,
	).Scan(&exists)
	return exists, err
}

func (s *Store) Create(ctx context.Context, accountID, publicationID uuid.UUID, p CreateParams) (*Source, error) {
	owns, err := s.publicationBelongsToAccount(ctx, accountID, publicationID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, ErrPublicationNF
	}

	interval := DefaultPollInterval(p.Type)
	if p.PollInterval != nil {
		interval = *p.PollInterval
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}

	row := s.pool.QueryRow(ctx, `
		insert into sources (publication_id, type, identifier, poll_interval, enabled)
		values ($1, $2, $3, $4, $5)
		returning id, publication_id, type, identifier, poll_interval, enabled, last_polled_at, created_at, updated_at
	`, publicationID, string(p.Type), p.Identifier, interval, enabled)

	src, err := scanSource(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return src, nil
}

func (s *Store) Get(ctx context.Context, accountID, publicationID, id uuid.UUID) (*Source, error) {
	row := s.pool.QueryRow(ctx, `
		select s.id, s.publication_id, s.type, s.identifier, s.poll_interval,
		       s.enabled, s.last_polled_at, s.created_at, s.updated_at
		from sources s
		join publications p on p.id = s.publication_id
		where s.id = $1 and s.publication_id = $2 and p.account_id = $3
	`, id, publicationID, accountID)
	src, err := scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return src, err
}

func (s *Store) List(ctx context.Context, accountID, publicationID uuid.UUID) ([]Source, error) {
	owns, err := s.publicationBelongsToAccount(ctx, accountID, publicationID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, ErrPublicationNF
	}

	rows, err := s.pool.Query(ctx, `
		select id, publication_id, type, identifier, poll_interval, enabled, last_polled_at, created_at, updated_at
		from sources
		where publication_id = $1
		order by created_at desc, id desc
	`, publicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		var src Source
		if err := rows.Scan(&src.ID, &src.PublicationID, &src.Type, &src.Identifier,
			&src.PollInterval, &src.Enabled, &src.LastPolledAt, &src.CreatedAt, &src.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) Update(ctx context.Context, accountID, publicationID, id uuid.UUID, p UpdateParams) (*Source, error) {
	row := s.pool.QueryRow(ctx, `
		update sources s set
		    identifier    = coalesce($4, s.identifier),
		    poll_interval = coalesce($5, s.poll_interval),
		    enabled       = coalesce($6, s.enabled),
		    updated_at    = now()
		from publications p
		where s.id = $1 and s.publication_id = $2 and p.id = s.publication_id and p.account_id = $3
		returning s.id, s.publication_id, s.type, s.identifier, s.poll_interval,
		          s.enabled, s.last_polled_at, s.created_at, s.updated_at
	`, id, publicationID, accountID, p.Identifier, p.PollInterval, p.Enabled)
	src, err := scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return src, nil
}

func (s *Store) Delete(ctx context.Context, accountID, publicationID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		delete from sources s
		using publications p
		where s.id = $1 and s.publication_id = $2 and p.id = s.publication_id and p.account_id = $3
	`, id, publicationID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSource(row pgx.Row) (*Source, error) {
	var src Source
	if err := row.Scan(&src.ID, &src.PublicationID, &src.Type, &src.Identifier,
		&src.PollInterval, &src.Enabled, &src.LastPolledAt, &src.CreatedAt, &src.UpdatedAt); err != nil {
		return nil, err
	}
	return &src, nil
}

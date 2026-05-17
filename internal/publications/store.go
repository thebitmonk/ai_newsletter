package publications

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("publication not found")

const (
	DefaultLimit = 25
	MaxLimit     = 100
)

type Publication struct {
	ID                  uuid.UUID     `json:"id"`
	AccountID           uuid.UUID     `json:"account_id"`
	Name                string        `json:"name"`
	Brief               string        `json:"brief"`
	Timezone            string        `json:"timezone"`
	CadenceRule         *string       `json:"cadence_rule,omitempty"`
	StoriesPerIssueMin  int           `json:"stories_per_issue_min"`
	StoriesPerIssueMax  int           `json:"stories_per_issue_max"`
	IntroEnabled        bool          `json:"intro_enabled"`
	CurationLeadTime    time.Duration `json:"curation_lead_time_ns"`
	ApprovalGateEnabled bool          `json:"approval_gate_enabled"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
}

type CreateParams struct {
	Name                string
	Brief               string
	Timezone            string
	CadenceRule         *string
	StoriesPerIssueMin  *int
	StoriesPerIssueMax  *int
	IntroEnabled        *bool
	CurationLeadTime    *time.Duration
	ApprovalGateEnabled *bool
}

type UpdateParams struct {
	Name                *string
	Brief               *string
	Timezone            *string
	CadenceRule         *string
	UnsetCadenceRule    bool
	StoriesPerIssueMin  *int
	StoriesPerIssueMax  *int
	IntroEnabled        *bool
	CurationLeadTime    *time.Duration
	ApprovalGateEnabled *bool
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Create(ctx context.Context, accountID uuid.UUID, p CreateParams) (*Publication, error) {
	row := s.pool.QueryRow(ctx, `
		insert into publications
		    (account_id, name, brief, timezone, cadence_rule,
		     stories_per_issue_min, stories_per_issue_max,
		     intro_enabled, curation_lead_time, approval_gate_enabled)
		values ($1, $2, $3, $4, $5,
		        coalesce($6, 3), coalesce($7, 7),
		        coalesce($8, true), coalesce($9, interval '24 hours'), coalesce($10, false))
		returning id, account_id, name, brief, timezone, cadence_rule,
		          stories_per_issue_min, stories_per_issue_max, intro_enabled,
		          curation_lead_time, approval_gate_enabled, created_at, updated_at
	`,
		accountID, p.Name, p.Brief, p.Timezone, p.CadenceRule,
		p.StoriesPerIssueMin, p.StoriesPerIssueMax,
		p.IntroEnabled, p.CurationLeadTime, p.ApprovalGateEnabled,
	)
	return scanPublication(row)
}

func (s *Store) Get(ctx context.Context, accountID, id uuid.UUID) (*Publication, error) {
	row := s.pool.QueryRow(ctx, `
		select id, account_id, name, brief, timezone, cadence_rule,
		       stories_per_issue_min, stories_per_issue_max, intro_enabled,
		       curation_lead_time, approval_gate_enabled, created_at, updated_at
		from publications
		where id = $1 and account_id = $2
	`, id, accountID)
	pub, err := scanPublication(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return pub, err
}

type Cursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	return &c, nil
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID, cursor *Cursor, limit int) ([]Publication, *Cursor, error) {
	if limit <= 0 || limit > MaxLimit {
		limit = DefaultLimit
	}
	args := []any{accountID, limit + 1}
	q := `
		select id, account_id, name, brief, timezone, cadence_rule,
		       stories_per_issue_min, stories_per_issue_max, intro_enabled,
		       curation_lead_time, approval_gate_enabled, created_at, updated_at
		from publications
		where account_id = $1
	`
	if cursor != nil {
		q += ` and (created_at, id) < ($3, $4)`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	q += ` order by created_at desc, id desc limit $2`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]Publication, 0, limit+1)
	for rows.Next() {
		var pub Publication
		if err := rows.Scan(
			&pub.ID, &pub.AccountID, &pub.Name, &pub.Brief, &pub.Timezone, &pub.CadenceRule,
			&pub.StoriesPerIssueMin, &pub.StoriesPerIssueMax, &pub.IntroEnabled,
			&pub.CurationLeadTime, &pub.ApprovalGateEnabled, &pub.CreatedAt, &pub.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		out = append(out, pub)
	}
	if rows.Err() != nil {
		return nil, nil, rows.Err()
	}

	var next *Cursor
	if len(out) > limit {
		last := out[limit-1]
		next = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		out = out[:limit]
	}
	return out, next, nil
}

func (s *Store) Update(ctx context.Context, accountID, id uuid.UUID, p UpdateParams) (*Publication, error) {
	row := s.pool.QueryRow(ctx, `
		update publications set
		    name                  = coalesce($3, name),
		    brief                 = coalesce($4, brief),
		    timezone              = coalesce($5, timezone),
		    cadence_rule          = case
		                                when $6::boolean then null
		                                else coalesce($7, cadence_rule)
		                            end,
		    stories_per_issue_min = coalesce($8, stories_per_issue_min),
		    stories_per_issue_max = coalesce($9, stories_per_issue_max),
		    intro_enabled         = coalesce($10, intro_enabled),
		    curation_lead_time    = coalesce($11, curation_lead_time),
		    approval_gate_enabled = coalesce($12, approval_gate_enabled),
		    updated_at            = now()
		where id = $1 and account_id = $2
		returning id, account_id, name, brief, timezone, cadence_rule,
		          stories_per_issue_min, stories_per_issue_max, intro_enabled,
		          curation_lead_time, approval_gate_enabled, created_at, updated_at
	`,
		id, accountID,
		p.Name, p.Brief, p.Timezone,
		p.UnsetCadenceRule, p.CadenceRule,
		p.StoriesPerIssueMin, p.StoriesPerIssueMax,
		p.IntroEnabled, p.CurationLeadTime, p.ApprovalGateEnabled,
	)
	pub, err := scanPublication(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return pub, err
}

func (s *Store) Delete(ctx context.Context, accountID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`delete from publications where id = $1 and account_id = $2`,
		id, accountID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanPublication(row pgx.Row) (*Publication, error) {
	var pub Publication
	if err := row.Scan(
		&pub.ID, &pub.AccountID, &pub.Name, &pub.Brief, &pub.Timezone, &pub.CadenceRule,
		&pub.StoriesPerIssueMin, &pub.StoriesPerIssueMax, &pub.IntroEnabled,
		&pub.CurationLeadTime, &pub.ApprovalGateEnabled, &pub.CreatedAt, &pub.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &pub, nil
}

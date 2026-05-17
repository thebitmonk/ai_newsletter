// Package candidates is the per-Publication pool of items fetched from
// Sources that have not yet (and may never) become Stories. See CONTEXT.md
// entry "Candidate" and ADR-0004.
package candidates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Item is what a SourceAdapter returns when polling — the raw shape of a
// single feed entry before it is persisted as a Candidate. See
// internal/sourceadapter for adapter implementations.
type Item struct {
	SourceItemID string          // unique within a Source — used for dedup
	URL          string          // canonical link
	Title        string          // optional
	Raw          json.RawMessage // adapter-defined opaque blob
	PublishedAt  time.Time       // best-effort, may be zero
}

// Candidate is a persisted Item awaiting selection into an Issue.
type Candidate struct {
	ID            uuid.UUID
	PublicationID uuid.UUID
	SourceID      uuid.UUID
	SourceItemID  string
	URL           string
	Title         string
	Raw           json.RawMessage
	FetchedAt     time.Time
	ExpiresAt     time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Upsert writes items to the pool. Duplicates by (publication_id,
// source_item_id) are silent no-ops — the unique index handles dedup.
// Returns the number of rows actually inserted (i.e. new Candidates).
func (s *Store) Upsert(ctx context.Context, publicationID, sourceID uuid.UUID, items []Item, ttl time.Duration) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	expires := time.Now().Add(ttl)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inserted int
	for _, it := range items {
		raw := it.Raw
		if len(raw) == 0 {
			raw = []byte("null")
		}
		tag, err := tx.Exec(ctx, `
			insert into candidates
			    (publication_id, source_id, source_item_id, url, title, raw_content, expires_at)
			values ($1, $2, $3, $4, $5, $6, $7)
			on conflict (publication_id, source_item_id) do nothing
		`, publicationID, sourceID, it.SourceItemID, it.URL, nullableString(it.Title), raw, expires)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() == 1 {
			inserted++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return inserted, nil
}

// ListActive returns all live Candidates for a Publication (expires_at > now),
// newest first. `since` further filters to fetched_at >= since (pass zero
// time to disable). Order is fetched_at desc, id desc.
func (s *Store) ListActive(ctx context.Context, publicationID uuid.UUID, since time.Time) ([]Candidate, error) {
	q := `
		select id, publication_id, source_id, source_item_id, url, title,
		       raw_content, fetched_at, expires_at
		from candidates
		where publication_id = $1 and expires_at > now()
	`
	args := []any{publicationID}
	if !since.IsZero() {
		q += ` and fetched_at >= $2`
		args = append(args, since)
	}
	q += ` order by fetched_at desc, id desc`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		var title *string
		if err := rows.Scan(&c.ID, &c.PublicationID, &c.SourceID, &c.SourceItemID,
			&c.URL, &title, &c.Raw, &c.FetchedAt, &c.ExpiresAt); err != nil {
			return nil, err
		}
		if title != nil {
			c.Title = *title
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ErrNotFound is returned by GetByURL when no live Candidate matches.
var ErrNotFound = errors.New("candidate not found")

// ListCursor pages forward through ListForAccount. Encodes the last row's
// (fetched_at, id) tuple — same pattern as issues.ListCursor.
type ListCursor struct {
	FetchedAt time.Time `json:"f"`
	ID        uuid.UUID `json:"i"`
}

// PoolItem is a Candidate joined with enough Source info to render a
// pool-view row without a per-row second query.
type PoolItem struct {
	Candidate
	SourceType       string
	SourceIdentifier string
}

// ListForAccount returns one page of live (non-expired) Candidates for one
// Publication, account-scoped via the parent. Returns one extra row
// internally to derive next_cursor cleanly.
func (s *Store) ListForAccount(
	ctx context.Context,
	accountID, publicationID uuid.UUID,
	cursor *ListCursor,
	limit int,
) ([]PoolItem, *ListCursor, error) {
	q := `
		select c.id, c.publication_id, c.source_id, c.source_item_id, c.url, c.title,
		       c.raw_content, c.fetched_at, c.expires_at,
		       s.type, s.identifier
		from candidates c
		join sources s on s.id = c.source_id
		join publications p on p.id = c.publication_id
		where c.publication_id = $1 and p.account_id = $2 and c.expires_at > now()
	`
	args := []any{publicationID, accountID}
	idx := 3
	if cursor != nil {
		q += fmt.Sprintf(" and (c.fetched_at, c.id) < ($%d, $%d)", idx, idx+1)
		args = append(args, cursor.FetchedAt, cursor.ID)
		idx += 2
	}
	q += fmt.Sprintf(" order by c.fetched_at desc, c.id desc limit $%d", idx)
	args = append(args, limit+1)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]PoolItem, 0, limit+1)
	for rows.Next() {
		var it PoolItem
		var title *string
		if err := rows.Scan(&it.ID, &it.PublicationID, &it.SourceID, &it.SourceItemID,
			&it.URL, &title, &it.Raw, &it.FetchedAt, &it.ExpiresAt,
			&it.SourceType, &it.SourceIdentifier); err != nil {
			return nil, nil, err
		}
		if title != nil {
			it.Title = *title
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *ListCursor
	if len(out) > limit {
		last := out[limit-1]
		next = &ListCursor{FetchedAt: last.FetchedAt, ID: last.ID}
		out = out[:limit]
	}
	return out, next, nil
}

// GetByURL looks up the most recent live Candidate for (publicationID, url).
// Used by Story regenerate to recover the originating Candidate for a Story
// node. Returns ErrNotFound if no live row matches (e.g. expired from pool).
func (s *Store) GetByURL(ctx context.Context, publicationID uuid.UUID, url string) (*Candidate, error) {
	row := s.pool.QueryRow(ctx, `
		select id, publication_id, source_id, source_item_id, url, title,
		       raw_content, fetched_at, expires_at
		from candidates
		where publication_id = $1 and url = $2 and expires_at > now()
		order by fetched_at desc
		limit 1
	`, publicationID, url)
	var c Candidate
	var title *string
	if err := row.Scan(&c.ID, &c.PublicationID, &c.SourceID, &c.SourceItemID,
		&c.URL, &title, &c.Raw, &c.FetchedAt, &c.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if title != nil {
		c.Title = *title
	}
	return &c, nil
}

// ExpireOlderThan deletes Candidates whose expires_at <= cutoff.
// Returns the number of rows deleted.
func (s *Store) ExpireOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `delete from candidates where expires_at <= $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

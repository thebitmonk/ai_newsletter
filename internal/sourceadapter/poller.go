package sourceadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nsqio/go-nsq"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

// PollTopic is the NSQ topic on which the source poller listens.
var PollTopic = nsqx.Topic("source", "poll")

// PollMessage is the JSON payload of a source.poll NSQ message. Per
// ADR-0015's rule 3, payloads carry only IDs.
type PollMessage struct {
	SourceID string `json:"source_id"`
}

// CandidateTTL is the lifetime of a Candidate in the pool. Per ADR-0004 the
// spec is max(cadence_interval * 2, 7 days). v1 uses a flat 7 days; the
// cadence-aware computation is a follow-up once we have a clearer signal
// that the simple version causes problems.
const CandidateTTL = 7 * 24 * time.Hour

// Poller fetches from Sources via the registry and writes Candidates.
type Poller struct {
	pool       *pgxpool.Pool
	registry   *Registry
	candidates *candidates.Store
	producer   *nsqx.Producer
}

func NewPoller(pool *pgxpool.Pool, reg *Registry, c *candidates.Store, p *nsqx.Producer) *Poller {
	return &Poller{pool: pool, registry: reg, candidates: c, producer: p}
}

// HandleMessage is the NSQ handler. Always returns nil so NSQ does not
// requeue — the deferred re-publish below is the retry mechanism.
func (p *Poller) HandleMessage(msg *nsq.Message) error {
	var m PollMessage
	if err := json.Unmarshal(msg.Body, &m); err != nil {
		log.Printf("poller: bad payload: %v", err)
		return nil
	}
	id, err := uuid.Parse(m.SourceID)
	if err != nil {
		log.Printf("poller: bad source_id: %v", err)
		return nil
	}
	if err := p.PollOnce(context.Background(), id); err != nil {
		log.Printf("poller: PollOnce(%s): %v", id, err)
	}
	return nil
}

// PollOnce performs one poll cycle for a Source: load → fetch → upsert →
// update last_polled_at → enqueue next poll. Idempotent through the
// candidates dedup constraint.
func (p *Poller) PollOnce(ctx context.Context, sourceID uuid.UUID) error {
	src, err := p.loadSource(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("load source: %w", err)
	}
	if src == nil {
		log.Printf("poller: source %s gone, dropping", sourceID)
		return nil // no reschedule
	}
	if !src.Enabled {
		log.Printf("poller: source %s disabled, dropping", sourceID)
		return nil
	}

	adapter, err := p.registry.Get(src.Type)
	if err != nil {
		log.Printf("poller: source %s type %s: %v", sourceID, src.Type, err)
		return p.scheduleNext(src) // still reschedule — adapter might be added later
	}

	items, err := adapter.Fetch(ctx, *src)
	if err != nil {
		log.Printf("poller: source %s fetch: %v", sourceID, err)
		// fall through to reschedule
	} else {
		if _, err := p.candidates.Upsert(ctx, src.PublicationID, src.ID, items, CandidateTTL); err != nil {
			log.Printf("poller: source %s upsert: %v", sourceID, err)
		}
	}

	if _, err := p.pool.Exec(ctx,
		`update sources set last_polled_at = now() where id = $1`, src.ID); err != nil {
		log.Printf("poller: source %s update last_polled_at: %v", sourceID, err)
	}
	return p.scheduleNext(src)
}

// Bootstrap publishes an immediate source.poll for the given Source.
// Called from the source-creation handler and the startup supervisor.
func (p *Poller) Bootstrap(sourceID uuid.UUID) error {
	body, _ := json.Marshal(PollMessage{SourceID: sourceID.String()})
	return p.producer.Publish(PollTopic, body)
}

func (p *Poller) scheduleNext(src *sources.Source) error {
	body, _ := json.Marshal(PollMessage{SourceID: src.ID.String()})
	return p.producer.PublishDeferred(PollTopic, src.PollInterval, body)
}

func (p *Poller) loadSource(ctx context.Context, id uuid.UUID) (*sources.Source, error) {
	row := p.pool.QueryRow(ctx, `
		select id, publication_id, type, identifier, poll_interval,
		       enabled, last_polled_at, created_at, updated_at
		from sources where id = $1
	`, id)
	var s sources.Source
	if err := row.Scan(&s.ID, &s.PublicationID, &s.Type, &s.Identifier,
		&s.PollInterval, &s.Enabled, &s.LastPolledAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// Supervisor scans for Sources that need a bootstrap poll — never polled,
// or last polled longer ago than their poll_interval — and publishes initial
// polls for each. Idempotent: rerunning soon after has no effect because
// last_polled_at is updated by the poller within one cycle.
type Supervisor struct {
	pool   *pgxpool.Pool
	poller *Poller
}

func NewSupervisor(pool *pgxpool.Pool, poller *Poller) *Supervisor {
	return &Supervisor{pool: pool, poller: poller}
}

// RunOnce bootstraps all Sources that look overdue. Safe to call at startup
// and periodically thereafter.
func (s *Supervisor) RunOnce(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		select id from sources
		where enabled = true
		  and (last_polled_at is null
		       or last_polled_at + poll_interval < now())
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, id := range ids {
		if err := s.poller.Bootstrap(id); err != nil {
			log.Printf("supervisor: bootstrap %s: %v", id, err)
		}
	}
	return nil
}

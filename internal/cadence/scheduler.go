package cadence

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nsqio/go-nsq"

	"github.com/thebitmonk/ai_newsletter/internal/curation"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
)

// TickTopic is the NSQ topic the scheduler self-publishes to drive itself.
// Each tick handler enqueues the next tick deferred TickInterval into the
// future, so the loop is self-sustaining.
var TickTopic = nsqx.Topic("cadence", "tick")

// TickInterval is how often the scheduler re-runs (the deferred publish delay).
const TickInterval = time.Minute

// LookaheadWindow is how far into the future the scheduler materialises slots.
// The PRD spec is "next 7 days".
const LookaheadWindow = 7 * 24 * time.Hour

// MaxSlotsPerRunPerPublication caps how many slots one Publication can
// materialise per tick (safety bound against pathological rules).
const MaxSlotsPerRunPerPublication = 200

// Producer is the subset of nsqx.Producer the scheduler uses, defined here so
// tests can substitute a fake. *nsqx.Producer satisfies it.
type Producer interface {
	Publish(topic string, body []byte) error
	PublishDeferred(topic string, delay time.Duration, body []byte) error
}

// Scheduler runs the cadence-driven slot materialisation loop.
type Scheduler struct {
	pool     *pgxpool.Pool
	producer Producer
	issues   *issues.Store
	clock    func() time.Time
}

func NewScheduler(pool *pgxpool.Pool, producer Producer, issuesStore *issues.Store) *Scheduler {
	return &Scheduler{
		pool:     pool,
		producer: producer,
		issues:   issuesStore,
		clock:    time.Now,
	}
}

// WithClock allows tests to inject a deterministic clock.
func (s *Scheduler) WithClock(clk func() time.Time) *Scheduler {
	s.clock = clk
	return s
}

// HandleTick is the NSQ message handler. It publishes the next tick first
// (so the loop self-sustains even if work below fails) then runs scheduling.
func (s *Scheduler) HandleTick(_ *nsq.Message) error {
	if s.producer != nil {
		if err := s.producer.PublishDeferred(TickTopic, TickInterval, []byte("tick")); err != nil {
			log.Printf("cadence scheduler: enqueue next tick: %v", err)
		}
	}
	if err := s.RunOnce(context.Background()); err != nil {
		log.Printf("cadence scheduler: run: %v", err)
	}
	return nil
}

// RunOnce performs one scheduling pass: for every Publication with a cadence
// rule, materialise any missing planned Issues in the lookahead window and
// enqueue a curation.start for each newly-created Issue at
// scheduled_at - curation_lead_time.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	now := s.clock()
	until := now.Add(LookaheadWindow)

	rows, err := s.pool.Query(ctx, `
		select id, cadence_rule, timezone, curation_lead_time
		from publications
		where cadence_rule is not null
	`)
	if err != nil {
		return fmt.Errorf("scheduler: list publications: %w", err)
	}
	defer rows.Close()

	type pubRow struct {
		id              uuid.UUID
		rule            string
		timezone        string
		curationLeadTime time.Duration
	}
	var pubs []pubRow
	for rows.Next() {
		var p pubRow
		if err := rows.Scan(&p.id, &p.rule, &p.timezone, &p.curationLeadTime); err != nil {
			return fmt.Errorf("scheduler: scan publication: %w", err)
		}
		pubs = append(pubs, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, p := range pubs {
		tz, err := time.LoadLocation(p.timezone)
		if err != nil {
			log.Printf("scheduler: pub %s has invalid timezone %q: %v", p.id, p.timezone, err)
			continue
		}
		slots, err := SlotsBetween(p.rule, tz, now, until, MaxSlotsPerRunPerPublication)
		if err != nil {
			log.Printf("scheduler: pub %s expand: %v", p.id, err)
			continue
		}
		for _, slot := range slots {
			iss, created, err := s.issues.CreatePlanned(ctx, p.id, slot)
			if err != nil {
				log.Printf("scheduler: pub %s create planned %s: %v", p.id, slot, err)
				continue
			}
			if !created {
				continue
			}
			if err := s.enqueueCuration(iss.ID, slot, p.curationLeadTime, now); err != nil {
				log.Printf("scheduler: pub %s enqueue curation for issue %s: %v",
					p.id, iss.ID, err)
			}
		}
	}
	return nil
}

// enqueueCuration publishes curation.start to fire `lead` before scheduledAt.
// If the firing time is already in the past, publishes immediately.
func (s *Scheduler) enqueueCuration(issueID uuid.UUID, scheduledAt time.Time, lead time.Duration, now time.Time) error {
	if s.producer == nil {
		return nil
	}
	body, _ := json.Marshal(curation.StartMessage{IssueID: issueID.String()})
	fireAt := scheduledAt.Add(-lead)
	delay := fireAt.Sub(now)
	if delay <= 0 {
		return s.producer.Publish(curation.StartTopic, body)
	}
	return s.producer.PublishDeferred(curation.StartTopic, delay, body)
}

// Bootstrap publishes an initial tick so the scheduler loop starts on app
// startup. Safe to call repeatedly — the work is idempotent.
func (s *Scheduler) Bootstrap() error {
	if s.producer == nil {
		return nil
	}
	return s.producer.Publish(TickTopic, []byte("bootstrap"))
}

package server_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/thebitmonk/ai_newsletter/internal/cadence"
	"github.com/thebitmonk/ai_newsletter/internal/curation"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

// fakeProducer captures publishes for assertions.
type fakeProducer struct {
	mu       sync.Mutex
	immediate []fakeMsg
	deferred  []fakeMsg
}

type fakeMsg struct {
	topic string
	delay time.Duration
	body  []byte
}

func (f *fakeProducer) Publish(topic string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.immediate = append(f.immediate, fakeMsg{topic: topic, body: body})
	return nil
}
func (f *fakeProducer) PublishDeferred(topic string, delay time.Duration, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deferred = append(f.deferred, fakeMsg{topic: topic, delay: delay, body: body})
	return nil
}

// TestScheduler_RunOnce_MaterialisesPlannedIssues exercises the scheduler
// against a real Postgres without needing NSQ — the publish-tick step is the
// only NSQ touchpoint and is independent of slot materialisation.
func TestScheduler_RunOnce_MaterialisesPlannedIssues(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "sched-mat@example.com")

	// Publication with weekly Mon 09:00 ET cadence.
	_, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":         "Weekly Mon NY",
		"timezone":     "America/New_York",
		"cadence_rule": "FREQ=WEEKLY;BYDAY=MO;BYHOUR=9;BYMINUTE=0;BYSECOND=0",
	}, token)
	pubID := body["id"].(string)

	store := issues.NewStore(testPool)
	sched := cadence.NewScheduler(testPool, nil, store).
		WithClock(func() time.Time {
			// Sat Mar 7 2026 12:00 UTC — 3 upcoming Mondays in the 7-day window
			// would be Mar 9, Mar 16; only the first falls inside 7 days.
			return time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
		})
	if err := sched.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Validate via the in-process store rather than HTTP since the Issue read
	// API is a later slice (#11).
	pubUUID := parseUUID(t, pubID)
	all, err := store.ListByPublication(context.Background(), pubUUID)
	if err != nil {
		t.Fatalf("ListByPublication: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 planned issue in 7-day window, got %d", len(all))
	}
	want := time.Date(2026, 3, 9, 13, 0, 0, 0, time.UTC) // Mon Mar 9 09:00 ET post-DST
	if !all[0].ScheduledAt.Equal(want) {
		t.Errorf("scheduled_at: want %s, got %s", want, all[0].ScheduledAt)
	}
	if all[0].State != issues.StatePlanned {
		t.Errorf("state: want planned, got %s", all[0].State)
	}
}

func TestScheduler_RunOnce_Idempotent(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "sched-idem@example.com")

	_, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":         "P",
		"timezone":     "UTC",
		"cadence_rule": "FREQ=DAILY;BYHOUR=12;BYMINUTE=0;BYSECOND=0",
	}, token)
	pubID := body["id"].(string)
	pubUUID := parseUUID(t, pubID)

	store := issues.NewStore(testPool)
	sched := cadence.NewScheduler(testPool, nil, store).
		WithClock(func() time.Time {
			return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		})

	if err := sched.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce 1: %v", err)
	}
	first, _ := store.ListByPublication(context.Background(), pubUUID)
	if err := sched.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce 2: %v", err)
	}
	second, _ := store.ListByPublication(context.Background(), pubUUID)
	if len(first) != len(second) || len(first) == 0 {
		t.Fatalf("second run should not change row count: first=%d second=%d", len(first), len(second))
	}
}

func TestScheduler_RunOnce_EnqueuesCurationAtLeadTime(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "sched-enqueue@example.com")

	// Daily cadence, 24h curation lead time (default).
	_, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":         "Daily",
		"timezone":     "UTC",
		"cadence_rule": "FREQ=DAILY;BYHOUR=12;BYMINUTE=0;BYSECOND=0",
	}, token)
	pubID := body["id"].(string)
	pubUUID := parseUUID(t, pubID)

	store := issues.NewStore(testPool)
	prod := &fakeProducer{}
	// Clock at noon 2026-05-01 UTC. With daily cadence at 12:00 UTC and 24h
	// lead time, the slot at 2026-05-02 12:00 has curation fire-time of
	// 2026-05-01 12:00 — exactly now, so we expect immediate publish on the
	// freshly-created slot for May 2.
	clkNow := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	sched := cadence.NewScheduler(testPool, prod, store).
		WithClock(func() time.Time { return clkNow })

	if err := sched.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	all, _ := store.ListByPublication(context.Background(), pubUUID)
	if len(all) == 0 {
		t.Fatal("expected planned issues created")
	}
	// Expect one curation publish per created Issue.
	total := len(prod.immediate) + len(prod.deferred)
	if total != len(all) {
		t.Fatalf("curation publishes: want %d, got %d (immediate=%d deferred=%d)",
			len(all), total, len(prod.immediate), len(prod.deferred))
	}

	// At least the first (May 2 slot) should fire immediately (fireAt = May 1 12:00 = clkNow).
	if len(prod.immediate) == 0 {
		t.Errorf("expected at least one immediate curation publish for May 2 slot")
	}

	// All deferred should target curation.start.
	for _, m := range prod.deferred {
		if m.topic != curation.StartTopic {
			t.Errorf("deferred topic: want %s, got %s", curation.StartTopic, m.topic)
		}
	}
	for _, m := range prod.immediate {
		if m.topic != curation.StartTopic {
			t.Errorf("immediate topic: want %s, got %s", curation.StartTopic, m.topic)
		}
	}
}

func TestScheduler_RunOnce_DoesNotReenqueueExistingIssues(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "sched-noreq@example.com")

	_, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":         "P",
		"timezone":     "UTC",
		"cadence_rule": "FREQ=DAILY;BYHOUR=9;BYMINUTE=0;BYSECOND=0",
	}, token)
	pubUUID := parseUUID(t, body["id"].(string))

	store := issues.NewStore(testPool)
	prod := &fakeProducer{}
	clkNow := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	sched := cadence.NewScheduler(testPool, prod, store).
		WithClock(func() time.Time { return clkNow })

	if err := sched.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce 1: %v", err)
	}
	first := len(prod.immediate) + len(prod.deferred)
	if first == 0 {
		t.Fatal("first run produced no curation publishes")
	}

	// Second run should publish nothing additional — all slots already exist.
	if err := sched.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce 2: %v", err)
	}
	second := len(prod.immediate) + len(prod.deferred)
	if second != first {
		t.Errorf("second run added publishes: was %d, now %d", first, second)
	}

	// And no new Issues.
	all, _ := store.ListByPublication(context.Background(), pubUUID)
	if len(all) != first {
		t.Errorf("issue count: want %d (one per publish), got %d", first, len(all))
	}
}

func TestScheduler_RunOnce_NoCadenceRule_NoOp(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "sched-noop@example.com")

	// Publication with no cadence_rule.
	_, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": "Ad hoc only", "timezone": "UTC",
	}, token)
	pubUUID := parseUUID(t, body["id"].(string))

	store := issues.NewStore(testPool)
	sched := cadence.NewScheduler(testPool, nil, store).
		WithClock(func() time.Time { return time.Now() })
	if err := sched.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	all, _ := store.ListByPublication(context.Background(), pubUUID)
	if len(all) != 0 {
		t.Fatalf("expected 0 issues for cadence-less publication, got %d", len(all))
	}
}

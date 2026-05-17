package server_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/thebitmonk/ai_newsletter/internal/cadence"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

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

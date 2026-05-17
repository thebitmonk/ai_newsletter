package cadence_test

import (
	"testing"
	"time"

	"github.com/thebitmonk/ai_newsletter/internal/cadence"
)

func mustLoadLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation %s: %v", name, err)
	}
	return loc
}

func TestNextNSlots_WeeklyMonday9amET(t *testing.T) {
	ny := mustLoadLoc(t, "America/New_York")
	// Anchor on a Sunday afternoon so the next Monday 9am ET is tomorrow.
	after := time.Date(2026, 3, 8, 14, 0, 0, 0, ny) // Sun Mar 8, 2026, 14:00 ET
	rule := "FREQ=WEEKLY;BYDAY=MO;BYHOUR=9;BYMINUTE=0;BYSECOND=0"

	slots, err := cadence.NextNSlots(rule, ny, after, 3)
	if err != nil {
		t.Fatalf("NextNSlots: %v", err)
	}
	if len(slots) != 3 {
		t.Fatalf("want 3 slots, got %d", len(slots))
	}

	want := []time.Time{
		time.Date(2026, 3, 9, 13, 0, 0, 0, time.UTC),  // post-DST jump, ET=UTC-4
		time.Date(2026, 3, 16, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 23, 13, 0, 0, 0, time.UTC),
	}
	for i, w := range want {
		if !slots[i].Equal(w) {
			t.Errorf("slot %d: want %s, got %s", i, w, slots[i])
		}
	}
}

func TestNextNSlots_SpansDSTSpringForward(t *testing.T) {
	ny := mustLoadLoc(t, "America/New_York")
	// 2026 US spring-forward: Sunday 2026-03-08 at 02:00 → 03:00 ET.
	// Anchor just before the previous Sunday so slots[0] is pre-DST and
	// slots[1] straddles the change.
	after := time.Date(2026, 2, 28, 23, 59, 0, 0, ny) // Sat Feb 28 23:59 ET (pre-DST)
	rule := "FREQ=WEEKLY;BYDAY=SU;BYHOUR=9;BYMINUTE=0;BYSECOND=0"

	slots, err := cadence.NextNSlots(rule, ny, after, 3)
	if err != nil {
		t.Fatalf("NextNSlots: %v", err)
	}
	if len(slots) != 3 {
		t.Fatalf("want 3 slots, got %d", len(slots))
	}
	// Pre-DST Sunday: ET=UTC-5, so 09:00 ET = 14:00 UTC.
	wantPre := time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC)
	// Post-DST Sunday: ET=UTC-4, so 09:00 ET = 13:00 UTC.
	wantPost := time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC)
	if !slots[0].Equal(wantPre) {
		t.Errorf("pre-DST slot: want %s, got %s", wantPre, slots[0])
	}
	if !slots[1].Equal(wantPost) {
		t.Errorf("post-DST slot: want %s, got %s", wantPost, slots[1])
	}
}

func TestNextNSlots_SpansDSTFallBack(t *testing.T) {
	ny := mustLoadLoc(t, "America/New_York")
	// 2026 US fall-back: Sunday 2026-11-01 at 02:00 → 01:00 ET.
	after := time.Date(2026, 10, 24, 23, 59, 0, 0, ny) // Sat Oct 24 23:59 ET (pre-fall-back)
	rule := "FREQ=WEEKLY;BYDAY=SU;BYHOUR=9;BYMINUTE=0;BYSECOND=0"

	slots, err := cadence.NextNSlots(rule, ny, after, 2)
	if err != nil {
		t.Fatalf("NextNSlots: %v", err)
	}
	// Pre-fall-back Sun: ET=UTC-4, so 09:00 ET = 13:00 UTC.
	wantPre := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	// Post-fall-back Sun: ET=UTC-5, so 09:00 ET = 14:00 UTC.
	wantPost := time.Date(2026, 11, 1, 14, 0, 0, 0, time.UTC)
	if !slots[0].Equal(wantPre) {
		t.Errorf("pre-fall-back slot: want %s, got %s", wantPre, slots[0])
	}
	if !slots[1].Equal(wantPost) {
		t.Errorf("post-fall-back slot: want %s, got %s", wantPost, slots[1])
	}
}

func TestNextNSlots_InvalidRule(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	_, err := cadence.NextNSlots("NOT A REAL RRULE", loc, time.Now(), 1)
	if err == nil {
		t.Fatal("expected error on invalid rule")
	}
}

func TestNextNSlots_NilTZ(t *testing.T) {
	_, err := cadence.NextNSlots("FREQ=WEEKLY;BYDAY=MO", nil, time.Now(), 1)
	if err == nil {
		t.Fatal("expected error on nil tz")
	}
}

func TestNextNSlots_ZeroN(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	_, err := cadence.NextNSlots("FREQ=WEEKLY;BYDAY=MO", loc, time.Now(), 0)
	if err == nil {
		t.Fatal("expected error on n=0")
	}
}

func TestNextNSlots_EmptyRule(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	_, err := cadence.NextNSlots("", loc, time.Now(), 1)
	if err == nil {
		t.Fatal("expected error on empty rule")
	}
}

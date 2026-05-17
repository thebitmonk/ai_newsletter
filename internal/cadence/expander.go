// Package cadence is the pure-function home for everything that converts a
// Publication's RRULE + timezone into concrete UTC slot times.
//
// Per ADR-0014 (per-Publication timezone, UTC storage), rrule interpretation
// uses the Publication's IANA timezone, and returned times are always UTC for
// storage as `timestamptz`.
package cadence

import (
	"errors"
	"fmt"
	"time"

	"github.com/teambition/rrule-go"
)

// NextNSlots returns the next n slot times strictly after `after`, interpreting
// the RRULE in the supplied IANA location. Returned times are UTC.
//
// Pure function — no I/O.
func NextNSlots(rule string, tz *time.Location, after time.Time, n int) ([]time.Time, error) {
	if rule == "" {
		return nil, errors.New("cadence: rule is empty")
	}
	if tz == nil {
		return nil, errors.New("cadence: tz is nil")
	}
	if n <= 0 {
		return nil, errors.New("cadence: n must be positive")
	}

	// rrule-go interprets DTSTART (or the rule's own dtstart) in whatever
	// location it has. We force the rule's start to be in `tz` so weekly
	// "Monday 09:00" means 09:00 wall-clock in tz, surviving DST correctly.
	opts, err := rrule.StrToROptionInLocation(rule, tz)
	if err != nil {
		return nil, fmt.Errorf("cadence: parse rule: %w", err)
	}
	// If the rule has no explicit DTSTART, anchor it to `after` in `tz` so
	// the recurrence starts walking forward from the relevant point in time.
	if opts.Dtstart.IsZero() {
		opts.Dtstart = after.In(tz)
	}

	r, err := rrule.NewRRule(*opts)
	if err != nil {
		return nil, fmt.Errorf("cadence: build rule: %w", err)
	}

	// Walk forward from after, collecting up to n slots. We need a hard
	// upper bound on the rrule iteration to handle pathological rules.
	const maxLookahead = 10 * 365 * 24 * time.Hour // 10y safety
	until := after.Add(maxLookahead)

	out := make([]time.Time, 0, n)
	iter := r.Iterator()
	for {
		t, ok := iter()
		if !ok {
			break
		}
		if !t.After(after) {
			continue
		}
		if t.After(until) {
			break
		}
		out = append(out, t.UTC())
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// ValidateRule parses rule against tz and returns nil if it can produce slots.
func ValidateRule(rule string, tz *time.Location) error {
	_, err := NextNSlots(rule, tz, time.Now(), 1)
	return err
}

// SlotsBetween returns all slot times in (after, until], capped at maxN to
// guard against pathological rules. Returned times are UTC.
func SlotsBetween(rule string, tz *time.Location, after, until time.Time, maxN int) ([]time.Time, error) {
	if rule == "" {
		return nil, fmt.Errorf("cadence: rule is empty")
	}
	if tz == nil {
		return nil, fmt.Errorf("cadence: tz is nil")
	}
	if !until.After(after) {
		return nil, fmt.Errorf("cadence: until must be after `after`")
	}
	if maxN <= 0 {
		maxN = 1000
	}

	opts, err := rrule.StrToROptionInLocation(rule, tz)
	if err != nil {
		return nil, fmt.Errorf("cadence: parse rule: %w", err)
	}
	if opts.Dtstart.IsZero() {
		opts.Dtstart = after.In(tz)
	}

	r, err := rrule.NewRRule(*opts)
	if err != nil {
		return nil, fmt.Errorf("cadence: build rule: %w", err)
	}

	out := make([]time.Time, 0)
	iter := r.Iterator()
	for {
		t, ok := iter()
		if !ok {
			break
		}
		if !t.After(after) {
			continue
		}
		if t.After(until) {
			break
		}
		out = append(out, t.UTC())
		if len(out) >= maxN {
			break
		}
	}
	return out, nil
}

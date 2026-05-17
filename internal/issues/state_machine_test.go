package issues_test

import (
	"errors"
	"testing"

	"github.com/thebitmonk/ai_newsletter/internal/issues"
)

func TestTransition_ValidPaths(t *testing.T) {
	cases := []struct {
		from issues.State
		evt  issues.Event
		want issues.State
	}{
		{"", issues.EventCreatePlanned, issues.StatePlanned},
		{"", issues.EventCreateDraftedAd, issues.StateDrafted},
		{issues.StatePlanned, issues.EventCurationStart, issues.StateCurating},
		{issues.StateCurating, issues.EventCurationOK, issues.StateDrafted},
		{issues.StateCurating, issues.EventCurationError, issues.StateFailed},
		{issues.StateCurating, issues.EventNoCandidates, issues.StateSkipped},
		{issues.StateDrafted, issues.EventApprove, issues.StateApproved},
		{issues.StateDrafted, issues.EventApprovalExpired, issues.StateSkipped},
		{issues.StateDrafted, issues.EventSendStart, issues.StateSending},
		{issues.StateApproved, issues.EventSendStart, issues.StateSending},
		{issues.StateSending, issues.EventSendOK, issues.StateSent},
		{issues.StateSending, issues.EventSendError, issues.StateFailed},
		{issues.StatePlanned, issues.EventCancel, issues.StateSkipped},
		{issues.StateCurating, issues.EventCancel, issues.StateSkipped},
		{issues.StateDrafted, issues.EventCancel, issues.StateSkipped},
		{issues.StateApproved, issues.EventCancel, issues.StateSkipped},
		{issues.StateFailed, issues.EventRetry, issues.StatePlanned},
	}
	for _, tc := range cases {
		got, err := issues.Transition(tc.from, tc.evt)
		if err != nil {
			t.Errorf("Transition(%q, %q): unexpected err %v", tc.from, tc.evt, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%q, %q): want %q, got %q", tc.from, tc.evt, tc.want, got)
		}
	}
}

func TestTransition_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from issues.State
		evt  issues.Event
	}{
		{issues.StatePlanned, issues.EventApprove},        // can't approve from planned
		{issues.StateDrafted, issues.EventCurationStart},  // can't re-curate drafted
		{issues.StateSent, issues.EventCancel},            // can't cancel sent
		{issues.StateSent, issues.EventRetry},             // can't retry sent
		{issues.StateSkipped, issues.EventRetry},          // can't retry skipped
		{issues.StateCurating, issues.EventApprove},       // can't approve while curating
		{"", issues.EventCurationStart},                   // can't start curation with no Issue
	}
	for _, tc := range cases {
		_, err := issues.Transition(tc.from, tc.evt)
		var invalid *issues.ErrInvalidTransition
		if !errors.As(err, &invalid) {
			t.Errorf("Transition(%q, %q): expected ErrInvalidTransition, got %v",
				tc.from, tc.evt, err)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	for _, s := range []issues.State{issues.StateSent, issues.StateFailed, issues.StateSkipped} {
		if !issues.IsTerminal(s) {
			t.Errorf("%q should be terminal", s)
		}
	}
	for _, s := range []issues.State{
		issues.StatePlanned, issues.StateCurating, issues.StateDrafted,
		issues.StateApproved, issues.StateSending,
	} {
		if issues.IsTerminal(s) {
			t.Errorf("%q should not be terminal", s)
		}
	}
}

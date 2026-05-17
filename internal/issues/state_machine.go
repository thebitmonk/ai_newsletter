// Package issues holds the Issue domain — the per-Publication dated sends.
// See CONTEXT.md for the glossary entry and ADR-0007 for the state machine.
package issues

import "fmt"

// State is the lifecycle state of an Issue. See ADR-0007.
type State string

const (
	StatePlanned  State = "planned"
	StateCurating State = "curating"
	StateDrafted  State = "drafted"
	StateApproved State = "approved"
	StateSending  State = "sending"
	StateSent     State = "sent"
	StateFailed   State = "failed"
	StateSkipped  State = "skipped"
)

// Event drives a State transition. The full set is defined here; the initial
// (#6) build only validates transitions for events relevant to creating
// `planned` Issues. Later slices add the rest.
type Event string

const (
	EventCreatePlanned   Event = "create_planned"   // → planned
	EventCreateDraftedAd Event = "create_drafted_ad_hoc" // → drafted (ad-hoc Issue)
	EventCurationStart   Event = "curation_start"
	EventCurationOK      Event = "curation_ok"
	EventCurationError   Event = "curation_error"
	EventNoCandidates    Event = "no_candidates"
	EventApprove         Event = "approve"
	EventSendStart       Event = "send_start"
	EventSendOK          Event = "send_ok"
	EventSendError       Event = "send_error"
	EventApprovalExpired Event = "approval_expired"
	EventCancel          Event = "cancel"
	EventRetry           Event = "retry"
)

// ErrInvalidTransition is returned by Transition for any (state, event) that
// is not in the table below.
type ErrInvalidTransition struct {
	From  State
	Event Event
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid transition: state=%q event=%q", e.From, e.Event)
}

// Transition resolves the next State for a given (currentState, event) per
// ADR-0007. Pure function — no I/O, no time dependence, no guards beyond the
// state-machine table. Callers (e.g. the approval-window freeze) layer their
// own guards on top.
//
// For events not yet wired up at this slice (curation, send, approval), the
// table still has the entries — adding the worker that emits each event is
// done in later slices. Listing them all here keeps the state machine the
// single source of truth.
func Transition(current State, event Event) (State, error) {
	if next, ok := table[key{current, event}]; ok {
		return next, nil
	}
	return "", &ErrInvalidTransition{From: current, Event: event}
}

type key struct {
	state State
	event Event
}

var table = map[key]State{
	// Initial Issue creation (no prior state).
	{"", EventCreatePlanned}:   StatePlanned,
	{"", EventCreateDraftedAd}: StateDrafted,

	// Curation.
	{StatePlanned, EventCurationStart}:  StateCurating,
	{StateCurating, EventCurationOK}:    StateDrafted,
	{StateCurating, EventCurationError}: StateFailed,
	{StateCurating, EventNoCandidates}:  StateSkipped,

	// Approval-gated path.
	{StateDrafted, EventApprove}:         StateApproved,
	{StateDrafted, EventApprovalExpired}: StateSkipped,

	// Send (both paths).
	{StateDrafted, EventSendStart}:  StateSending,
	{StateApproved, EventSendStart}: StateSending,
	{StateSending, EventSendOK}:     StateSent,
	{StateSending, EventSendError}:  StateFailed,

	// Owner-initiated cancel from any non-terminal state.
	{StatePlanned, EventCancel}:   StateSkipped,
	{StateCurating, EventCancel}:  StateSkipped,
	{StateDrafted, EventCancel}:   StateSkipped,
	{StateApproved, EventCancel}:  StateSkipped,

	// Recovery.
	{StateFailed, EventRetry}: StatePlanned,
}

// IsTerminal reports whether further transitions out of s are possible.
func IsTerminal(s State) bool {
	return s == StateSent || s == StateFailed || s == StateSkipped
}

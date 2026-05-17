// Package sourceadapter defines the SourceAdapter contract for fetching
// items from external Sources, plus a registry the poller uses to dispatch
// by source type. Per-type implementations live in subpackages.
package sourceadapter

import (
	"context"
	"errors"
	"fmt"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

// SourceAdapter knows how to fetch a list of Items from a Source of one type.
type SourceAdapter interface {
	Fetch(ctx context.Context, source sources.Source) ([]candidates.Item, error)
}

// FetchError wraps an adapter-side failure for the poller. The poller logs
// these but does not requeue — the next scheduled poll will retry naturally.
type FetchError struct {
	Wrapped error
	Code    string // adapter-defined, e.g. "rate_limited", "not_found", "parse"
}

func (e *FetchError) Error() string {
	if e.Code == "" {
		return e.Wrapped.Error()
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Wrapped.Error())
}
func (e *FetchError) Unwrap() error { return e.Wrapped }

// Registry maps source types to adapters. The poller dispatches by type
// through Get — no caller switches on type directly.
type Registry struct {
	byType map[sources.Type]SourceAdapter
}

func NewRegistry() *Registry {
	return &Registry{byType: make(map[sources.Type]SourceAdapter)}
}

func (r *Registry) Register(t sources.Type, a SourceAdapter) {
	r.byType[t] = a
}

func (r *Registry) Get(t sources.Type) (SourceAdapter, error) {
	a, ok := r.byType[t]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for source type %q", t)
	}
	return a, nil
}

// ErrAdapterNotFound is returned by Registry.Get when no adapter is registered.
var ErrAdapterNotFound = errors.New("no adapter registered for source type")

// Package xhandle is a placeholder SourceAdapter for X/Twitter handles.
//
// As of v1 we have not chosen between (a) the paid X API and (b) a public
// nitter-style RSS bridge, so this adapter returns an empty Item slice and
// logs a one-shot warning on each Fetch. The adapter is still registered so
// owners can create X-type Sources today without errors; ingestion simply
// produces zero Candidates until a real implementation lands.
//
// To enable real ingestion later, swap this adapter for one that talks to
// the chosen backend. The Source schema and identifier validation already
// match either approach (handle without @).
package xhandle

import (
	"context"
	"log"
	"sync"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

type Adapter struct {
	warnOnce sync.Once
}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Fetch(_ context.Context, source sources.Source) ([]candidates.Item, error) {
	a.warnOnce.Do(func() {
		log.Printf("xhandle adapter is a stub — handle=%q will produce no Candidates until a real backend is wired up", source.Identifier)
	})
	return nil, nil
}

// Package rss implements SourceAdapter for RSS, Atom, and RDF feeds.
//
// source_item_id resolution order: entry.GUID, then entry.Link. Items lacking
// both are skipped (they have no stable dedup key).
package rss

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

// Adapter is the RSS/Atom SourceAdapter.
type Adapter struct {
	client *http.Client
	parser *gofeed.Parser
}

// New constructs an Adapter with a default 10-second HTTP timeout.
func New() *Adapter {
	c := &http.Client{Timeout: 10 * time.Second}
	p := gofeed.NewParser()
	p.Client = c
	return &Adapter{client: c, parser: p}
}

// Fetch retrieves the feed at source.Identifier and converts its entries into
// Items. Per-entry errors (missing GUID and Link) are skipped silently; a
// fetch-level failure (network, parse) is returned as a *FetchError.
func (a *Adapter) Fetch(ctx context.Context, source sources.Source) ([]candidates.Item, error) {
	if source.Type != sources.TypeRSS && source.Type != sources.TypeSubstack {
		return nil, &sourceadapter.FetchError{
			Code:    "wrong_type",
			Wrapped: fmt.Errorf("rss adapter received source type %q", source.Type),
		}
	}
	feed, err := a.parser.ParseURLWithContext(source.Identifier, ctx)
	if err != nil {
		return nil, &sourceadapter.FetchError{Code: "parse", Wrapped: err}
	}

	items := make([]candidates.Item, 0, len(feed.Items))
	for _, e := range feed.Items {
		id := e.GUID
		if id == "" {
			id = e.Link
		}
		if id == "" {
			continue // no stable dedup key — skip
		}
		url := e.Link
		if url == "" {
			url = id
		}

		raw, _ := json.Marshal(e)
		published := time.Time{}
		if e.PublishedParsed != nil {
			published = *e.PublishedParsed
		} else if e.UpdatedParsed != nil {
			published = *e.UpdatedParsed
		}

		items = append(items, candidates.Item{
			SourceItemID: id,
			URL:          url,
			Title:        e.Title,
			Raw:          raw,
			PublishedAt:  published,
		})
	}
	return items, nil
}

// Package rss implements SourceAdapter for RSS, Atom, and RDF feeds, and
// exposes a FetchFeed primitive that other adapters (youtube, substack,
// generic web) wrap when they ultimately resolve to an RSS/Atom source.
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

// Adapter is the RSS/Atom SourceAdapter and feed-fetch primitive.
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

// HTTPClient returns the adapter's HTTP client so wrapping adapters that need
// to make their own requests (e.g. the generic-web autodiscovery probe) can
// reuse the same timeout settings.
func (a *Adapter) HTTPClient() *http.Client { return a.client }

// FetchFeed fetches and parses the feed at url and returns its items as
// Candidate Items. The same function is used by Fetch (Source-driven) and by
// other adapters that resolve to an RSS URL.
func (a *Adapter) FetchFeed(ctx context.Context, url string) ([]candidates.Item, error) {
	feed, err := a.parser.ParseURLWithContext(url, ctx)
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
		linkURL := e.Link
		if linkURL == "" {
			linkURL = id
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
			URL:          linkURL,
			Title:        e.Title,
			Raw:          raw,
			PublishedAt:  published,
		})
	}
	return items, nil
}

// Fetch is the SourceAdapter contract. RSS uses source.Identifier directly;
// Substack appends /feed via the substack adapter (not this one).
func (a *Adapter) Fetch(ctx context.Context, source sources.Source) ([]candidates.Item, error) {
	if source.Type != sources.TypeRSS {
		return nil, &sourceadapter.FetchError{
			Code:    "wrong_type",
			Wrapped: fmt.Errorf("rss adapter received source type %q", source.Type),
		}
	}
	return a.FetchFeed(ctx, source.Identifier)
}

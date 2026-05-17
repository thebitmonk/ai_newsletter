// Package substack implements SourceAdapter for Substack publications. Every
// Substack publication exposes an RSS feed at <publication-url>/feed; this
// adapter appends that suffix (if not already present) and delegates to the
// rss adapter.
package substack

import (
	"context"
	"fmt"
	"strings"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

type feedFetcher interface {
	FetchFeed(ctx context.Context, url string) ([]candidates.Item, error)
}

type Adapter struct {
	feed feedFetcher
}

func New(feed feedFetcher) *Adapter {
	return &Adapter{feed: feed}
}

func (a *Adapter) Fetch(ctx context.Context, source sources.Source) ([]candidates.Item, error) {
	if source.Type != sources.TypeSubstack {
		return nil, &sourceadapter.FetchError{
			Code:    "wrong_type",
			Wrapped: fmt.Errorf("substack adapter received source type %q", source.Type),
		}
	}
	return a.feed.FetchFeed(ctx, feedURLFor(source.Identifier))
}

// feedURLFor accepts either the publication URL ("https://stratechery.com")
// or the feed URL ("https://stratechery.com/feed") and returns the feed URL.
func feedURLFor(identifier string) string {
	id := strings.TrimRight(identifier, "/")
	if strings.HasSuffix(id, "/feed") {
		return id
	}
	return id + "/feed"
}

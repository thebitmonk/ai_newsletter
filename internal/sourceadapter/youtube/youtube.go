// Package youtube implements SourceAdapter for YouTube channels via the
// public-by-default RSS feed at
// https://www.youtube.com/feeds/videos.xml?channel_id=<ID>. No API key required.
package youtube

import (
	"context"
	"fmt"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

// feedFetcher is the slice of RSSAdapter behaviour youtube needs. Defined here
// rather than importing the rss package to keep youtube test-driveable with a
// stub.
type feedFetcher interface {
	FetchFeed(ctx context.Context, url string) ([]candidates.Item, error)
}

// Adapter fetches a channel's uploads via the public YouTube RSS feed.
type Adapter struct {
	feed         feedFetcher
	feedBaseURL  string
}

// New constructs an Adapter using the supplied RSS fetcher. The fetcher is
// usually rss.New().FeedFetcher (or whatever exposes FetchFeed).
func New(feed feedFetcher) *Adapter {
	return &Adapter{
		feed:        feed,
		feedBaseURL: "https://www.youtube.com/feeds/videos.xml",
	}
}

// WithFeedBaseURL is for tests — override the YouTube feed host with a
// httptest.NewServer URL so the test does not hit YouTube.
func (a *Adapter) WithFeedBaseURL(u string) *Adapter {
	a.feedBaseURL = u
	return a
}

func (a *Adapter) Fetch(ctx context.Context, source sources.Source) ([]candidates.Item, error) {
	if source.Type != sources.TypeYouTubeChannel {
		return nil, &sourceadapter.FetchError{
			Code:    "wrong_type",
			Wrapped: fmt.Errorf("youtube adapter received source type %q", source.Type),
		}
	}
	url := fmt.Sprintf("%s?channel_id=%s", a.feedBaseURL, source.Identifier)
	return a.feed.FetchFeed(ctx, url)
}

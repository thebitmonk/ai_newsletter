// Package web implements SourceAdapter for arbitrary websites/blogs by
// discovering an RSS or Atom feed via <link rel="alternate"> in the page's
// HTML and delegating to the rss adapter. HTML scraping is intentionally
// out of scope — if no feed is discoverable, Fetch returns a typed error.
package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

type feedFetcher interface {
	FetchFeed(ctx context.Context, url string) ([]candidates.Item, error)
	HTTPClient() *http.Client
}

type Adapter struct {
	feed feedFetcher
}

func New(feed feedFetcher) *Adapter {
	return &Adapter{feed: feed}
}

func (a *Adapter) Fetch(ctx context.Context, source sources.Source) ([]candidates.Item, error) {
	if source.Type != sources.TypeWeb {
		return nil, &sourceadapter.FetchError{
			Code:    "wrong_type",
			Wrapped: fmt.Errorf("web adapter received source type %q", source.Type),
		}
	}
	feedURL, err := a.discoverFeed(ctx, source.Identifier)
	if err != nil {
		return nil, err
	}
	return a.feed.FetchFeed(ctx, feedURL)
}

// discoverFeed fetches the page, scans for the first <link rel="alternate">
// pointing at an RSS or Atom feed, and resolves it to an absolute URL.
func (a *Adapter) discoverFeed(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", &sourceadapter.FetchError{Code: "request", Wrapped: err}
	}
	req.Header.Set("Accept", "text/html")

	resp, err := a.feed.HTTPClient().Do(req)
	if err != nil {
		return "", &sourceadapter.FetchError{Code: "http", Wrapped: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", &sourceadapter.FetchError{
			Code:    "http",
			Wrapped: fmt.Errorf("status %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return "", &sourceadapter.FetchError{Code: "read", Wrapped: err}
	}

	href := extractFeedLink(string(body))
	if href == "" {
		return "", &sourceadapter.FetchError{
			Code:    "no_feed",
			Wrapped: fmt.Errorf("no <link rel=\"alternate\"> feed discovered at %s", pageURL),
		}
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return "", &sourceadapter.FetchError{Code: "parse_base", Wrapped: err}
	}
	rel, err := url.Parse(href)
	if err != nil {
		return "", &sourceadapter.FetchError{Code: "parse_feed", Wrapped: err}
	}
	return base.ResolveReference(rel).String(), nil
}

// extractFeedLink walks the HTML tokens looking for the first <link> whose
// rel contains "alternate" and whose type is RSS or Atom. Returns the raw
// href (possibly relative) or "" if none.
func extractFeedLink(body string) string {
	tk := html.NewTokenizer(strings.NewReader(body))
	for {
		tt := tk.Next()
		if tt == html.ErrorToken {
			return ""
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		t := tk.Token()
		if t.Data != "link" {
			continue
		}
		var rel, typ, href string
		for _, a := range t.Attr {
			switch strings.ToLower(a.Key) {
			case "rel":
				rel = strings.ToLower(a.Val)
			case "type":
				typ = strings.ToLower(a.Val)
			case "href":
				href = a.Val
			}
		}
		if !strings.Contains(rel, "alternate") {
			continue
		}
		if typ == "application/rss+xml" || typ == "application/atom+xml" {
			return href
		}
	}
}

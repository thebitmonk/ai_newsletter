package rss_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/rss"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

const validRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example Feed</title>
    <link>https://example.com</link>
    <description>Test feed</description>
    <item>
      <title>Post One</title>
      <link>https://example.com/post-1</link>
      <guid>post-1-guid</guid>
      <pubDate>Wed, 06 May 2026 12:00:00 GMT</pubDate>
    </item>
    <item>
      <title>Post Two</title>
      <link>https://example.com/post-2</link>
      <guid>post-2-guid</guid>
      <pubDate>Thu, 07 May 2026 12:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

const validAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <link href="https://atom.example.com"/>
  <id>urn:uuid:atom-feed-id</id>
  <updated>2026-05-06T12:00:00Z</updated>
  <entry>
    <id>atom-entry-1</id>
    <title>Atom Post 1</title>
    <link href="https://atom.example.com/1"/>
    <updated>2026-05-06T12:00:00Z</updated>
  </entry>
</feed>`

const malformed = `<rss><channel><item>oops missing close tags`

func startFeedServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func makeSource(t string, url string) sources.Source {
	st := sources.TypeRSS
	if t == "substack" {
		st = sources.TypeSubstack
	}
	return sources.Source{
		ID:           uuid.New(),
		Type:         st,
		Identifier:   url,
		PollInterval: time.Hour,
	}
}

func TestRSSAdapter_ParsesValidRSS(t *testing.T) {
	srv := startFeedServer(t, validRSS, http.StatusOK)
	defer srv.Close()

	a := rss.New()
	items, err := a.Fetch(context.Background(), makeSource("rss", srv.URL))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].SourceItemID != "post-1-guid" || items[0].Title != "Post One" {
		t.Errorf("item[0] mismatch: %+v", items[0])
	}
	if items[1].URL != "https://example.com/post-2" {
		t.Errorf("item[1] URL mismatch: %s", items[1].URL)
	}
}

func TestRSSAdapter_ParsesAtom(t *testing.T) {
	srv := startFeedServer(t, validAtom, http.StatusOK)
	defer srv.Close()

	a := rss.New()
	items, err := a.Fetch(context.Background(), makeSource("rss", srv.URL))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].SourceItemID != "atom-entry-1" {
		t.Errorf("atom guid mismatch: %q", items[0].SourceItemID)
	}
}

func TestRSSAdapter_MalformedFeedReturnsError(t *testing.T) {
	srv := startFeedServer(t, malformed, http.StatusOK)
	defer srv.Close()

	a := rss.New()
	_, err := a.Fetch(context.Background(), makeSource("rss", srv.URL))
	if err == nil {
		t.Fatal("expected error on malformed feed")
	}
}

func TestRSSAdapter_HTTPError(t *testing.T) {
	srv := startFeedServer(t, "", http.StatusInternalServerError)
	defer srv.Close()

	a := rss.New()
	_, err := a.Fetch(context.Background(), makeSource("rss", srv.URL))
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestRSSAdapter_RejectsWrongType(t *testing.T) {
	src := sources.Source{
		ID:         uuid.New(),
		Type:       sources.TypeYouTubeChannel,
		Identifier: "UCxxxxxxxxxxxxxxxxxxxxxx",
	}
	_, err := rss.New().Fetch(context.Background(), src)
	if err == nil {
		t.Fatal("expected error on wrong source type")
	}
}

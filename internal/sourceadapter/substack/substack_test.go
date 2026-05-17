package substack_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/rss"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/substack"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

const substackFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Stratechery</title>
    <item>
      <title>Post</title>
      <link>https://stratechery.com/post-1</link>
      <guid>post-1</guid>
    </item>
  </channel>
</rss>`

func TestSubstack_AppendsFeed_PublicationURL(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(substackFeed))
	}))
	defer srv.Close()

	a := substack.New(rss.New())
	source := sources.Source{
		ID:         uuid.New(),
		Type:       sources.TypeSubstack,
		Identifier: srv.URL, // just the publication URL
	}
	_, err := a.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.HasSuffix(seenPath, "/feed") {
		t.Errorf("expected feed suffix, got %q", seenPath)
	}
}

func TestSubstack_NoDoubleAppend_FeedURL(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(substackFeed))
	}))
	defer srv.Close()

	a := substack.New(rss.New())
	source := sources.Source{
		ID:         uuid.New(),
		Type:       sources.TypeSubstack,
		Identifier: srv.URL + "/feed", // already a feed URL
	}
	_, err := a.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if strings.HasSuffix(seenPath, "/feed/feed") {
		t.Errorf("/feed was doubled: %q", seenPath)
	}
}

func TestSubstack_WrongType(t *testing.T) {
	a := substack.New(rss.New())
	_, err := a.Fetch(context.Background(), sources.Source{Type: sources.TypeRSS})
	if err == nil {
		t.Fatal("expected error on wrong type")
	}
}

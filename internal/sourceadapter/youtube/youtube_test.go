package youtube_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/rss"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/youtube"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

const ytFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Channel Name</title>
  <entry>
    <id>yt:video:abc</id>
    <title>Video A</title>
    <link href="https://www.youtube.com/watch?v=abc"/>
    <updated>2026-05-06T12:00:00Z</updated>
  </entry>
</feed>`

func TestYouTube_ConstructsCorrectFeedURL(t *testing.T) {
	var seenPath, seenQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(ytFeed))
	}))
	defer srv.Close()

	a := youtube.New(rss.New()).WithFeedBaseURL(srv.URL + "/feeds/videos.xml")
	source := sources.Source{
		ID:         uuid.New(),
		Type:       sources.TypeYouTubeChannel,
		Identifier: "UCAAAAAAAAAAAAAAAAAAAAAA",
	}
	items, err := a.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if seenPath != "/feeds/videos.xml" {
		t.Errorf("path: got %q", seenPath)
	}
	if seenQuery != "channel_id=UCAAAAAAAAAAAAAAAAAAAAAA" {
		t.Errorf("query: got %q", seenQuery)
	}
	if len(items) != 1 || items[0].SourceItemID != "yt:video:abc" {
		t.Errorf("items: %+v", items)
	}
}

func TestYouTube_WrongType(t *testing.T) {
	a := youtube.New(rss.New())
	_, err := a.Fetch(context.Background(), sources.Source{
		Type:       sources.TypeRSS,
		Identifier: "https://x.com/feed",
	})
	if err == nil {
		t.Fatal("expected error on wrong type")
	}
}

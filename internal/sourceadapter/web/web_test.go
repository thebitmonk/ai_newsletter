package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/rss"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/web"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>x</title>
    <item><title>post</title><link>https://blog.example.com/p1</link><guid>p1</guid></item>
  </channel>
</rss>`

func TestWeb_DiscoversFeedFromHTML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head>
			<link rel="alternate" type="application/rss+xml" href="/feed.xml">
		</head><body>hello</body></html>`))
	})
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(feedXML))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := web.New(rss.New())
	source := sources.Source{
		ID:         uuid.New(),
		Type:       sources.TypeWeb,
		Identifier: srv.URL,
	}
	items, err := a.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 || items[0].SourceItemID != "p1" {
		t.Errorf("items: %+v", items)
	}
}

func TestWeb_NoFeed_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>no feed here</body></html>`))
	}))
	defer srv.Close()

	a := web.New(rss.New())
	_, err := a.Fetch(context.Background(), sources.Source{
		Type:       sources.TypeWeb,
		Identifier: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error when no feed discoverable")
	}
}

func TestWeb_AtomTypeAlsoDiscovered(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>
			<link rel="alternate" type="application/atom+xml" href="/atom">
		</head></html>`))
	})
	atomFeed := `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
		<entry><id>a1</id><title>a</title><link href="https://x/a"/><updated>2026-01-01T00:00:00Z</updated></entry>
	</feed>`
	mux.HandleFunc("/atom", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(atomFeed))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := web.New(rss.New())
	items, err := a.Fetch(context.Background(), sources.Source{
		Type:       sources.TypeWeb,
		Identifier: srv.URL,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
}

func TestWeb_WrongType(t *testing.T) {
	_, err := web.New(rss.New()).Fetch(context.Background(), sources.Source{
		Type: sources.TypeRSS,
	})
	if err == nil {
		t.Fatal("expected error on wrong type")
	}
}

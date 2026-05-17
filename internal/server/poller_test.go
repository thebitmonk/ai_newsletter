package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter"
	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/rss"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

// mustProducer connects to the test NSQD (default localhost:5150) or skips.
// The poller needs a real producer because its publish-next-deferred step
// returns the error from NSQ — tests would fail spuriously with a fake.
func mustProducer(t *testing.T) *nsqx.Producer {
	t.Helper()
	addr := os.Getenv("TEST_NSQD_TCP_ADDR")
	if addr == "" {
		addr = "localhost:5150"
	}
	p, err := nsqx.NewProducer(addr)
	if err != nil {
		t.Skipf("SKIP: nsqd producer at %s: %v", addr, err)
	}
	// Sanity-check by publishing a healthcheck message.
	if err := p.Publish("__poller_test_healthcheck__", []byte("ping")); err != nil {
		t.Skipf("SKIP: nsqd unreachable at %s: %v", addr, err)
	}
	return p
}

const candidateFeedRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>https://example.com</link>
    <description>x</description>
    <item>
      <title>Item A</title>
      <link>https://example.com/a</link>
      <guid>item-a</guid>
    </item>
    <item>
      <title>Item B</title>
      <link>https://example.com/b</link>
      <guid>item-b</guid>
    </item>
  </channel>
</rss>`

// -----------------------------------------------------------------------------
// CandidateStore
// -----------------------------------------------------------------------------

func TestCandidateStore_Upsert_Dedups(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "cand-dedup@example.com")

	pubID := makePub(t, r, token, "P")
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed")

	store := candidates.NewStore(testPool)
	items := []candidates.Item{
		{SourceItemID: "x-1", URL: "https://x/1", Title: "x1", Raw: json.RawMessage(`{}`)},
		{SourceItemID: "x-2", URL: "https://x/2", Title: "x2", Raw: json.RawMessage(`{}`)},
	}
	pubUUID := parseUUID(t, pubID)
	srcUUID := parseUUID(t, srcID)
	n1, err := store.Upsert(context.Background(), pubUUID, srcUUID, items, time.Hour)
	if err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	if n1 != 2 {
		t.Fatalf("first upsert inserted=%d want 2", n1)
	}

	// Same items again — should insert nothing.
	n2, err := store.Upsert(context.Background(), pubUUID, srcUUID, items, time.Hour)
	if err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second upsert inserted=%d want 0", n2)
	}

	// Mixed: one new, one repeat.
	mix := append(items, candidates.Item{SourceItemID: "x-3", URL: "https://x/3", Title: "x3", Raw: json.RawMessage(`{}`)})
	n3, err := store.Upsert(context.Background(), pubUUID, srcUUID, mix, time.Hour)
	if err != nil {
		t.Fatalf("upsert 3: %v", err)
	}
	if n3 != 1 {
		t.Fatalf("mixed upsert inserted=%d want 1", n3)
	}
}

func TestCandidateStore_ListActive_FiltersExpired(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "cand-list@example.com")

	pubID := makePub(t, r, token, "P")
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed")
	pubUUID := parseUUID(t, pubID)
	srcUUID := parseUUID(t, srcID)

	store := candidates.NewStore(testPool)
	_, _ = store.Upsert(context.Background(), pubUUID, srcUUID,
		[]candidates.Item{{SourceItemID: "live", URL: "https://x/l", Raw: json.RawMessage(`{}`)}},
		1*time.Hour,
	)
	// Use a negative TTL to force an expired row.
	_, _ = store.Upsert(context.Background(), pubUUID, srcUUID,
		[]candidates.Item{{SourceItemID: "dead", URL: "https://x/d", Raw: json.RawMessage(`{}`)}},
		-1*time.Hour,
	)

	got, err := store.ListActive(context.Background(), pubUUID, time.Time{})
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 active candidate, got %d", len(got))
	}
	if got[0].SourceItemID != "live" {
		t.Fatalf("expected live, got %s", got[0].SourceItemID)
	}
}

func TestCandidateStore_ExpireOlderThan_Deletes(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "cand-expire@example.com")

	pubID := makePub(t, r, token, "P")
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed")
	pubUUID := parseUUID(t, pubID)
	srcUUID := parseUUID(t, srcID)

	store := candidates.NewStore(testPool)
	_, _ = store.Upsert(context.Background(), pubUUID, srcUUID,
		[]candidates.Item{{SourceItemID: "old", URL: "u", Raw: json.RawMessage(`{}`)}}, -time.Hour,
	)
	_, _ = store.Upsert(context.Background(), pubUUID, srcUUID,
		[]candidates.Item{{SourceItemID: "fresh", URL: "u2", Raw: json.RawMessage(`{}`)}}, time.Hour,
	)

	deleted, err := store.ExpireOlderThan(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("want 1 deleted, got %d", deleted)
	}
	active, _ := store.ListActive(context.Background(), pubUUID, time.Time{})
	if len(active) != 1 || active[0].SourceItemID != "fresh" {
		t.Fatalf("want only 'fresh' to remain, got %+v", active)
	}
}

// -----------------------------------------------------------------------------
// Poller — exercises PollOnce against a real feed; NSQ producer is exercised
// only at the publish-next step, which we don't assert here (covered by the
// nsqx integration tests). The producer is unused if PollOnce errors before
// it; if it succeeds it will try to PublishDeferred. We give it a real
// producer pointed at the project NSQ for the test that needs it; the
// remaining tests don't need the publish to succeed.
// -----------------------------------------------------------------------------

func TestPoller_PollOnce_FetchesAndPersistsCandidates(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "poller-fetch@example.com")

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(candidateFeedRSS))
	}))
	defer feed.Close()

	pubID := makePub(t, r, token, "P")
	srcID := makeSrc(t, r, token, pubID, "rss", feed.URL)

	prod := mustProducer(t)
	defer prod.Stop()

	store := candidates.NewStore(testPool)
	reg := sourceadapter.NewRegistry()
	reg.Register(sources.TypeRSS, rss.New())
	poller := sourceadapter.NewPoller(testPool, reg, store, prod)

	if err := poller.PollOnce(context.Background(), parseUUID(t, srcID)); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	got, err := store.ListActive(context.Background(), parseUUID(t, pubID), time.Time{})
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(got))
	}

	// Second poll: dedup'd, no growth.
	if err := poller.PollOnce(context.Background(), parseUUID(t, srcID)); err != nil {
		t.Fatalf("PollOnce 2: %v", err)
	}
	got2, _ := store.ListActive(context.Background(), parseUUID(t, pubID), time.Time{})
	if len(got2) != 2 {
		t.Fatalf("second poll grew candidates: want 2, got %d", len(got2))
	}

	// last_polled_at updated.
	var lastPolled *time.Time
	_ = testPool.QueryRow(context.Background(),
		`select last_polled_at from sources where id = $1`, parseUUID(t, srcID)).Scan(&lastPolled)
	if lastPolled == nil {
		t.Fatal("last_polled_at not updated")
	}
}

func TestPoller_PollOnce_DisabledSource_NoOp(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "poller-disabled@example.com")

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(candidateFeedRSS))
	}))
	defer feed.Close()

	pubID := makePub(t, r, token, "P")
	srcID := makeSrc(t, r, token, pubID, "rss", feed.URL)
	// Disable the source.
	_, _ = doJSON(t, r, http.MethodPatch,
		"/api/v1/publications/"+pubID+"/sources/"+srcID,
		map[string]any{"enabled": false}, token)

	prod := mustProducer(t)
	defer prod.Stop()
	store := candidates.NewStore(testPool)
	reg := sourceadapter.NewRegistry()
	reg.Register(sources.TypeRSS, rss.New())
	poller := sourceadapter.NewPoller(testPool, reg, store, prod)

	if err := poller.PollOnce(context.Background(), parseUUID(t, srcID)); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	got, _ := store.ListActive(context.Background(), parseUUID(t, pubID), time.Time{})
	if len(got) != 0 {
		t.Fatalf("disabled source should produce no candidates, got %d", len(got))
	}
}

func TestPoller_PollOnce_GoneSource_NoOp(t *testing.T) {
	prod := mustProducer(t)
	defer prod.Stop()
	store := candidates.NewStore(testPool)
	reg := sourceadapter.NewRegistry()
	reg.Register(sources.TypeRSS, rss.New())
	poller := sourceadapter.NewPoller(testPool, reg, store, prod)

	if err := poller.PollOnce(context.Background(), uuid.New()); err != nil {
		t.Fatalf("PollOnce on gone source returned error: %v", err)
	}
}

func TestSupervisor_BootstrapsOverdueSources(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "supervisor@example.com")

	pubID := makePub(t, r, token, "P")
	_ = makeSrc(t, r, token, pubID, "rss", "https://example.com/feed")

	prod := mustProducer(t)
	defer prod.Stop()
	store := candidates.NewStore(testPool)
	reg := sourceadapter.NewRegistry()
	reg.Register(sources.TypeRSS, rss.New())
	poller := sourceadapter.NewPoller(testPool, reg, store, prod)
	sup := sourceadapter.NewSupervisor(testPool, poller)

	// Should run without error against a never-polled source.
	if err := sup.RunOnce(context.Background()); err != nil {
		t.Fatalf("supervisor: %v", err)
	}
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func makeSrc(t *testing.T, r http.Handler, token, pubID, srcType, identifier string) string {
	t.Helper()
	_, body := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources",
		map[string]any{"type": srcType, "identifier": identifier},
		token)
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("makeSrc failed: %v", body)
	}
	return id
}


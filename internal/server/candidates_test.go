package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
)

// seedCandidatesIntoPool inserts n candidates against the given (pub, src)
// with the given TTL, returning their ids.
func seedCandidatesIntoPool(t *testing.T, pubID, srcID uuid.UUID, n int, ttl time.Duration) {
	t.Helper()
	store := candidates.NewStore(testPool)
	items := make([]candidates.Item, n)
	for i := range items {
		items[i] = candidates.Item{
			SourceItemID: "pool-" + uuid.NewString(),
			URL:          "https://example.com/pool-item-" + uuid.NewString(),
			Title:        "Pool item",
			Raw:          json.RawMessage(`{}`),
		}
		// Stagger fetched_at so cursor pagination has a meaningful order.
		time.Sleep(time.Millisecond)
	}
	if _, err := store.Upsert(context.Background(), pubID, srcID, items, ttl); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}

func TestCandidatesList_HappyPath(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pool-happy@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed.xml")
	seedCandidatesIntoPool(t, parseUUID(t, pubID), parseUUID(t, srcID), 3, time.Hour)

	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/candidates", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	items := body["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["source_type"] != "rss" {
		t.Errorf("source_type missing on row: %v", first)
	}
}

func TestCandidatesList_CursorPagination(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pool-page@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed.xml")
	seedCandidatesIntoPool(t, parseUUID(t, pubID), parseUUID(t, srcID), 5, time.Hour)

	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/candidates?limit=2", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("page 1: %d", w.Code)
	}
	p1 := body["items"].([]any)
	if len(p1) != 2 {
		t.Fatalf("page 1 want 2, got %d", len(p1))
	}
	cur := body["next_cursor"].(string)

	w, body = doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/candidates?limit=2&cursor="+cur, nil, token)
	p2 := body["items"].([]any)
	if len(p2) != 2 {
		t.Fatalf("page 2 want 2, got %d", len(p2))
	}
	cur = body["next_cursor"].(string)

	w, body = doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/candidates?limit=2&cursor="+cur, nil, token)
	p3 := body["items"].([]any)
	if len(p3) != 1 {
		t.Fatalf("page 3 want 1, got %d", len(p3))
	}
	if body["next_cursor"] != nil {
		t.Errorf("expected null next_cursor at end, got %v", body["next_cursor"])
	}
}

func TestCandidatesList_OmitsExpired(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pool-exp@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed.xml")
	seedCandidatesIntoPool(t, parseUUID(t, pubID), parseUUID(t, srcID), 2, time.Hour)
	// One already-expired row.
	seedCandidatesIntoPool(t, parseUUID(t, pubID), parseUUID(t, srcID), 1, -time.Hour)

	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/candidates", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	items := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expired rows should be omitted: want 2, got %d", len(items))
	}
}

func TestCandidatesList_CrossAccount_EmptyItems(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, _ := signupAs(t, r, "pool-a@example.com")
	tokenB, _ := signupAs(t, r, "pool-b@example.com")
	pubA := makePubWithBrief(t, r, tokenA, "A", "brief", 1, 5)
	srcA := makeSrc(t, r, tokenA, pubA, "rss", "https://a.com/feed.xml")
	seedCandidatesIntoPool(t, parseUUID(t, pubA), parseUUID(t, srcA), 3, time.Hour)

	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubA+"/candidates", nil, tokenB)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	items := body["items"].([]any)
	if len(items) != 0 {
		t.Errorf("cross-account list should be empty, got %d items", len(items))
	}
}

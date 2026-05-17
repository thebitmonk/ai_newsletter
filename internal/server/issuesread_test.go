package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

// markDrafted is a test helper that pushes an Issue from planned to drafted
// with a synthetic body doc and 3 story nodes so story_count math is testable.
func markDrafted(t *testing.T, id uuid.UUID, subject string) {
	t.Helper()
	store := issues.NewStore(testPool)
	if _, err := store.ApplyTransition(context.Background(), id, issues.EventCurationStart, issues.TransitionUpdate{}); err != nil {
		t.Fatalf("planned->curating: %v", err)
	}
	doc := json.RawMessage(`{
		"type": "doc",
		"content": [
			{"type": "cover", "attrs": {"block": "cover"}},
			{"type": "story", "attrs": {"block": "story"}},
			{"type": "story", "attrs": {"block": "story"}},
			{"type": "story", "attrs": {"block": "story"}}
		]
	}`)
	cover := "https://cdn.example/cover.png"
	title := "Title"
	subj := subject
	if _, err := store.ApplyTransition(context.Background(), id, issues.EventCurationOK, issues.TransitionUpdate{
		Subject:  &subj,
		Title:    &title,
		CoverURL: &cover,
		BodyDoc:  doc,
	}); err != nil {
		t.Fatalf("curating->drafted: %v", err)
	}
}

func TestIssueGet_HappyPath(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "iget@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)

	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))
	markDrafted(t, issueID, "Hello")

	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/issues/"+issueID.String(), nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["state"] != "drafted" {
		t.Errorf("state: %v", body["state"])
	}
	if body["subject"] != "Hello" {
		t.Errorf("subject: %v", body["subject"])
	}
	if body["body_doc"] == nil {
		t.Error("body_doc missing in detail response")
	}
}

func TestIssueGet_CrossAccount_404(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	tokenA, _ := signupAs(t, r, "iget-a@example.com")
	tokenB, _ := signupAs(t, r, "iget-b@example.com")
	pubA := makePubWithBrief(t, r, tokenA, "A", "brief", 1, 5)
	issueID := seedPlanned(t, parseUUID(t, pubA), time.Now().Add(time.Hour))

	w, _ := doJSON(t, r, http.MethodGet,
		"/api/v1/issues/"+issueID.String(), nil, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-account get: want 404, got %d", w.Code)
	}
}

func TestIssueList_OmitsBodyDocAndComputesStoryCount(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "ilist@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)

	idPlanned := seedPlanned(t, pubUUID, time.Now().Add(2*time.Hour))
	idDrafted := seedPlanned(t, pubUUID, time.Now().Add(1*time.Hour))
	markDrafted(t, idDrafted, "drafted-one")

	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/issues", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	items := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}

	// items are scheduled-desc, so planned (later) comes first.
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["id"].(string) != idPlanned.String() {
		t.Errorf("first should be planned id %s, got %v", idPlanned, first["id"])
	}
	if _, present := first["body_doc"]; present {
		t.Error("list response must omit body_doc")
	}
	if int(first["story_count"].(float64)) != 0 {
		t.Errorf("planned story_count: want 0, got %v", first["story_count"])
	}
	if second["id"].(string) != idDrafted.String() {
		t.Errorf("second should be drafted id %s, got %v", idDrafted, second["id"])
	}
	if int(second["story_count"].(float64)) != 3 {
		t.Errorf("drafted story_count: want 3, got %v", second["story_count"])
	}
}

func TestIssueList_StateFilter(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "ifilter@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)

	idPlanned := seedPlanned(t, pubUUID, time.Now().Add(2*time.Hour))
	idDrafted := seedPlanned(t, pubUUID, time.Now().Add(1*time.Hour))
	markDrafted(t, idDrafted, "d")

	// drafted only
	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/issues?state=drafted", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	items := body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != idDrafted.String() {
		t.Errorf("drafted filter returned %v", items)
	}

	// multi-state filter
	w, body = doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/issues?state=planned&state=drafted", nil, token)
	items = body["items"].([]any)
	if len(items) != 2 {
		t.Errorf("multi-state filter: want 2, got %d", len(items))
	}
	_ = idPlanned
}

func TestIssueList_DateRange(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "irange@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)

	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		_ = seedPlanned(t, pubUUID, t0.Add(time.Duration(i)*24*time.Hour))
	}

	after := t0.Add(1*24*time.Hour - time.Minute).Format(time.RFC3339)
	before := t0.Add(3*24*time.Hour + time.Minute).Format(time.RFC3339)
	w, body := doJSON(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/publications/%s/issues?scheduled_after=%s&scheduled_before=%s",
			pubID, after, before), nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	items := body["items"].([]any)
	// Days 1, 2, 3 should be in range (3 items).
	if len(items) != 3 {
		t.Fatalf("date range: want 3 items, got %d", len(items))
	}
}

func TestIssueList_CursorPagination(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "ipage@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)

	t0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		_ = seedPlanned(t, pubUUID, t0.Add(time.Duration(i)*24*time.Hour))
	}

	// limit=2 first page
	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/issues?limit=2", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("page 1: %d", w.Code)
	}
	page1 := body["items"].([]any)
	if len(page1) != 2 {
		t.Fatalf("page 1: want 2, got %d", len(page1))
	}
	cursor1 := body["next_cursor"].(string)

	// page 2
	w, body = doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/issues?limit=2&cursor="+cursor1, nil, token)
	page2 := body["items"].([]any)
	if len(page2) != 2 {
		t.Fatalf("page 2: want 2, got %d", len(page2))
	}
	cursor2 := body["next_cursor"].(string)

	// page 3 has 1 item, no further cursor
	w, body = doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/issues?limit=2&cursor="+cursor2, nil, token)
	page3 := body["items"].([]any)
	if len(page3) != 1 {
		t.Fatalf("page 3: want 1, got %d", len(page3))
	}
	if body["next_cursor"] != nil {
		t.Errorf("expected null next_cursor on last page, got %v", body["next_cursor"])
	}
}

func TestIssueList_CrossAccount_404(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	tokenA, _ := signupAs(t, r, "ilist-a@example.com")
	tokenB, _ := signupAs(t, r, "ilist-b@example.com")
	pubA := makePubWithBrief(t, r, tokenA, "A", "brief", 1, 5)

	// B has no publication of that id. Listing should return empty items
	// rather than 404 (the list endpoint isn't a single-resource lookup), so
	// validate empty items instead.
	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubA+"/issues", nil, tokenB)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	items := body["items"].([]any)
	if len(items) != 0 {
		t.Errorf("expected empty items for cross-account list, got %d", len(items))
	}
}

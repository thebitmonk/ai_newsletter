package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/curation"
	"github.com/thebitmonk/ai_newsletter/internal/imagegen"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/llmclient"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

// -----------------------------------------------------------------------------
// stubs — exercise the curation pipeline without real LLM/image calls
// -----------------------------------------------------------------------------

type stubLLM struct {
	briefSeen []string // captured for assertions
	mu        sync.Mutex

	rankErr error
}

func (s *stubLLM) record(brief string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.briefSeen = append(s.briefSeen, brief)
}

func (s *stubLLM) Rank(_ context.Context, brief string, cs []candidates.Candidate, n int) ([]llmclient.ScoredCandidate, error) {
	s.record(brief)
	if s.rankErr != nil {
		return nil, s.rankErr
	}
	out := make([]llmclient.ScoredCandidate, 0, len(cs))
	for _, c := range cs {
		out = append(out, llmclient.ScoredCandidate{Candidate: c, Score: 0.9})
	}
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func (s *stubLLM) Summarize(_ context.Context, brief string, c candidates.Candidate) (llmclient.Summary, error) {
	s.record(brief)
	return llmclient.Summary{
		Headline: "Headline for " + c.Title,
		Body:     "Body discussion about " + c.URL,
	}, nil
}

func (s *stubLLM) Headline(_ context.Context, brief string, sums []llmclient.Summary) (llmclient.Headline, error) {
	s.record(brief)
	return llmclient.Headline{Subject: "Issue subject", Title: "Issue title"}, nil
}

func (s *stubLLM) Intro(_ context.Context, brief string, sums []llmclient.Summary) (string, error) {
	s.record(brief)
	return "Welcome back.", nil
}

type stubImage struct {
	briefSeen string
}

func (s *stubImage) Generate(_ context.Context, brief string, _ imagegen.PromptHints) (imagegen.ImageRef, error) {
	s.briefSeen = brief
	return imagegen.ImageRef{URL: "https://stub.example/cover.png", MIMEType: "image/png"}, nil
}

// -----------------------------------------------------------------------------
// fixtures
// -----------------------------------------------------------------------------

func makePubWithBrief(t *testing.T, r http.Handler, token, name, brief string, min, max int) string {
	t.Helper()
	_, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":                   name,
		"brief":                  brief,
		"timezone":               "UTC",
		"stories_per_issue_min":  min,
		"stories_per_issue_max":  max,
	}, token)
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("makePubWithBrief: %v", body)
	}
	return id
}

func seedCandidates(t *testing.T, pubID, srcID uuid.UUID, n int, ttl time.Duration) {
	t.Helper()
	store := candidates.NewStore(testPool)
	items := make([]candidates.Item, n)
	for i := range items {
		items[i] = candidates.Item{
			SourceItemID: fmt.Sprintf("seed-%d", i),
			URL:          fmt.Sprintf("https://seed.example/%d", i),
			Title:        fmt.Sprintf("Seed item %d", i),
			Raw:          json.RawMessage(`{}`),
		}
	}
	if _, err := store.Upsert(context.Background(), pubID, srcID, items, ttl); err != nil {
		t.Fatalf("seed candidates: %v", err)
	}
}

func seedPlanned(t *testing.T, pubID uuid.UUID, scheduledAt time.Time) uuid.UUID {
	t.Helper()
	store := issues.NewStore(testPool)
	iss, _, err := store.CreatePlanned(context.Background(), pubID, scheduledAt)
	if err != nil {
		t.Fatalf("seed planned: %v", err)
	}
	return iss.ID
}

// -----------------------------------------------------------------------------
// tests
// -----------------------------------------------------------------------------

func TestCuration_HappyPath_PlannedToDrafted(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "cur-happy@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "BRIEF: tech for engineers", 1, 5)
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed")

	pubUUID := parseUUID(t, pubID)
	srcUUID := parseUUID(t, srcID)
	seedCandidates(t, pubUUID, srcUUID, 3, time.Hour)
	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))

	stub := &stubLLM{}
	img := &stubImage{}
	store := issues.NewStore(testPool)
	cStore := candidates.NewStore(testPool)
	worker := curation.NewWorker(testPool, store, cStore, stub, stub, img)

	if err := worker.Curate(context.Background(), issueID); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	iss, err := store.Get(context.Background(), issueID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if iss.State != issues.StateDrafted {
		t.Fatalf("state: want drafted, got %s", iss.State)
	}
	if iss.Subject == nil || *iss.Subject != "Issue subject" {
		t.Errorf("subject: %v", iss.Subject)
	}
	if iss.CoverURL == nil || *iss.CoverURL != "https://stub.example/cover.png" {
		t.Errorf("cover url: %v", iss.CoverURL)
	}
	if len(iss.BodyDoc) == 0 {
		t.Fatal("body_doc is empty")
	}
	if stub.briefSeen[0] != "BRIEF: tech for engineers" {
		t.Errorf("brief not injected into ranker: %q", stub.briefSeen[0])
	}
	if img.briefSeen != "BRIEF: tech for engineers" {
		t.Errorf("brief not injected into image gen: %q", img.briefSeen)
	}
}

func TestCuration_ZeroCandidates_Skipped(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "cur-zero@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	_ = makeSrc(t, r, token, pubID, "rss", "https://example.com/feed")

	pubUUID := parseUUID(t, pubID)
	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))

	stub := &stubLLM{}
	img := &stubImage{}
	store := issues.NewStore(testPool)
	cStore := candidates.NewStore(testPool)
	worker := curation.NewWorker(testPool, store, cStore, stub, stub, img)

	if err := worker.Curate(context.Background(), issueID); err != nil {
		t.Fatalf("Curate: %v", err)
	}
	iss, _ := store.Get(context.Background(), issueID)
	if iss.State != issues.StateSkipped {
		t.Fatalf("state: want skipped, got %s", iss.State)
	}
}

func TestCuration_RankerError_Failed(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "cur-fail@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	srcID := makeSrc(t, r, token, pubID, "rss", "https://example.com/feed")

	pubUUID := parseUUID(t, pubID)
	srcUUID := parseUUID(t, srcID)
	seedCandidates(t, pubUUID, srcUUID, 2, time.Hour)
	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))

	stub := &stubLLM{rankErr: errors.New("openai 500")}
	img := &stubImage{}
	store := issues.NewStore(testPool)
	cStore := candidates.NewStore(testPool)
	worker := curation.NewWorker(testPool, store, cStore, stub, stub, img)

	if err := worker.Curate(context.Background(), issueID); err == nil {
		t.Fatal("expected Curate to surface ranker error")
	}

	iss, _ := store.Get(context.Background(), issueID)
	if iss.State != issues.StateFailed {
		t.Fatalf("state: want failed, got %s", iss.State)
	}
	if iss.FailedReason == nil || *iss.FailedReason == "" {
		t.Errorf("expected failed_reason to be set")
	}
}

func TestCuration_NotPlanned_NoOp(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "cur-noop@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)

	// Move the Issue past planned manually before Curate.
	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))
	store := issues.NewStore(testPool)
	_, err := store.ApplyTransition(context.Background(), issueID, issues.EventCancel, issues.TransitionUpdate{})
	if err != nil {
		t.Fatalf("manual cancel: %v", err)
	}

	stub := &stubLLM{}
	img := &stubImage{}
	cStore := candidates.NewStore(testPool)
	worker := curation.NewWorker(testPool, store, cStore, stub, stub, img)

	if err := worker.Curate(context.Background(), issueID); err != nil {
		t.Fatalf("Curate on non-planned: %v", err)
	}
	iss, _ := store.Get(context.Background(), issueID)
	if iss.State != issues.StateSkipped {
		t.Fatalf("state should still be skipped, got %s", iss.State)
	}
}

// -----------------------------------------------------------------------------
// HTTP trigger endpoint
// -----------------------------------------------------------------------------

func TestCurateEndpoint_HappyPath(t *testing.T) {
	truncate(t)
	var calledWith uuid.UUID
	trig := func(id uuid.UUID) error { calledWith = id; return nil }
	r := server.New(testPool, server.WithCurateTrigger(trig))

	token, _ := signupAs(t, r, "trig-happy@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)
	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/curate", nil, token)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %v", w.Code, body)
	}
	if calledWith != issueID {
		t.Errorf("trigger called with wrong id: %s vs %s", calledWith, issueID)
	}
}

func TestCurateEndpoint_CrossAccount_404(t *testing.T) {
	truncate(t)
	r := server.New(testPool, server.WithCurateTrigger(func(uuid.UUID) error { return nil }))
	tokenA, _ := signupAs(t, r, "trig-a@example.com")
	tokenB, _ := signupAs(t, r, "trig-b@example.com")
	pubA := makePubWithBrief(t, r, tokenA, "A", "brief", 1, 5)
	issueID := seedPlanned(t, parseUUID(t, pubA), time.Now().Add(time.Hour))

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/curate", nil, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestCurateEndpoint_WrongState_409(t *testing.T) {
	truncate(t)
	r := server.New(testPool, server.WithCurateTrigger(func(uuid.UUID) error { return nil }))
	token, _ := signupAs(t, r, "trig-state@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)
	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))

	store := issues.NewStore(testPool)
	_, _ = store.ApplyTransition(context.Background(), issueID, issues.EventCancel, issues.TransitionUpdate{})

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/curate", nil, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %v", w.Code, body)
	}
}

func TestCurateEndpoint_NoWorkerConfigured_503(t *testing.T) {
	truncate(t)
	r := server.New(testPool) // no trigger
	token, _ := signupAs(t, r, "trig-503@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	issueID := seedPlanned(t, parseUUID(t, pubID), time.Now().Add(time.Hour))

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/curate", nil, token)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

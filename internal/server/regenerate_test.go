package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/curation"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

// stubRegenerator simulates curation.Worker's regenerate ops for endpoint tests.
type stubRegenerator struct {
	storyErr    error
	coverErr    error
	storyCalls  int
	coverCalls  int
	lastStoryID uuid.UUID
}

func (s *stubRegenerator) RegenerateStorySummary(_ context.Context, iss *issues.Issue, storyID uuid.UUID) (*issues.Issue, error) {
	s.storyCalls++
	s.lastStoryID = storyID
	if s.storyErr != nil {
		return nil, s.storyErr
	}
	// Mutate body_doc minimally to prove the response carries through.
	updated := *iss
	doc := []byte(`{"type":"doc","content":[{"type":"story","attrs":{"storyId":"` + storyID.String() + `","block":"story"}}],"regenerated":"yes"}`)
	updated.BodyDoc = doc
	return &updated, nil
}

func (s *stubRegenerator) RegenerateCover(_ context.Context, iss *issues.Issue) (*issues.Issue, error) {
	s.coverCalls++
	if s.coverErr != nil {
		return nil, s.coverErr
	}
	updated := *iss
	newCover := "https://cdn.example/cover-regen.png"
	updated.CoverURL = &newCover
	return &updated, nil
}

// seedDrafted creates a planned Issue then pushes it to drafted with a
// body_doc containing one story node so we can target it for regen.
func seedDrafted(t *testing.T, pubID uuid.UUID, storyID uuid.UUID) uuid.UUID {
	t.Helper()
	store := issues.NewStore(testPool)
	iss, _, err := store.CreatePlanned(context.Background(), pubID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("seed planned: %v", err)
	}
	if _, err := store.ApplyTransition(context.Background(), iss.ID, issues.EventCurationStart, issues.TransitionUpdate{}); err != nil {
		t.Fatalf("planned->curating: %v", err)
	}
	doc := json.RawMessage(`{
		"type": "doc",
		"content": [
			{"type":"cover","attrs":{"block":"cover","src":"https://cdn/old.png"}},
			{"type":"story","attrs":{"block":"story","storyId":"` + storyID.String() + `","sourceUrl":"https://example.com/x"}}
		]
	}`)
	subj := "x"; title := "y"; cover := "https://cdn/old.png"
	if _, err := store.ApplyTransition(context.Background(), iss.ID, issues.EventCurationOK, issues.TransitionUpdate{
		Subject: &subj, Title: &title, CoverURL: &cover, BodyDoc: doc,
	}); err != nil {
		t.Fatalf("curating->drafted: %v", err)
	}
	return iss.ID
}

func TestRegenerate_NoRegeneratorWired_503(t *testing.T) {
	truncate(t)
	r := newServer(t) // no WithRegenerator
	token, _ := signupAs(t, r, "regen-503@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "summary"}, token)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 when no regenerator wired, got %d", w.Code)
	}
}

func TestRegenerate_SummaryHappyPath(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{}
	r := newServer(t, server.WithRegenerator(stub))
	token, _ := signupAs(t, r, "regen-sum@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "summary"}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if stub.storyCalls != 1 {
		t.Fatalf("story regen called %d times", stub.storyCalls)
	}
	if stub.lastStoryID != storyID {
		t.Errorf("story id passed to regen: got %s want %s", stub.lastStoryID, storyID)
	}
	if body["body_doc"] == nil {
		t.Error("response should include the updated body_doc")
	}
}

func TestRegenerate_CoverHappyPath(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{}
	r := newServer(t, server.WithRegenerator(stub))
	token, _ := signupAs(t, r, "regen-cov@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "image"}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if stub.coverCalls != 1 {
		t.Fatalf("cover regen called %d times", stub.coverCalls)
	}
	if body["cover_url"] != "https://cdn.example/cover-regen.png" {
		t.Errorf("cover url not updated: %v", body["cover_url"])
	}
}

func TestRegenerate_TypeOmitted_DefaultsToSummary(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{}
	r := newServer(t, server.WithRegenerator(stub))
	token, _ := signupAs(t, r, "regen-default@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if stub.storyCalls != 1 || stub.coverCalls != 0 {
		t.Errorf("expected summary regen by default: story=%d cover=%d", stub.storyCalls, stub.coverCalls)
	}
}

func TestRegenerate_InvalidType_400(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{}
	r := newServer(t, server.WithRegenerator(stub))
	token, _ := signupAs(t, r, "regen-bad@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "nonsense"}, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	if stub.storyCalls+stub.coverCalls != 0 {
		t.Error("regenerator should not have been called on invalid type")
	}
}

func TestRegenerate_CrossAccount_404(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{}
	r := newServer(t, server.WithRegenerator(stub))
	tokenA, _ := signupAs(t, r, "regen-a@example.com")
	tokenB, _ := signupAs(t, r, "regen-b@example.com")
	pubA := makePubWithBrief(t, r, tokenA, "A", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubA), storyID)

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "summary"}, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-account regen: want 404, got %d", w.Code)
	}
	if stub.storyCalls != 0 {
		t.Error("regenerator should not have been called for cross-account request")
	}
}

func TestRegenerate_WrongState_409(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{storyErr: curation.ErrWrongState}
	r := newServer(t, server.WithRegenerator(stub))
	token, _ := signupAs(t, r, "regen-state@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "summary"}, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %v", w.Code, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["code"] != "wrong_state" {
		t.Errorf("want wrong_state code, got %v", errBody)
	}
}

func TestRegenerate_StoryNotFound_404(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{storyErr: curation.ErrStoryNodeNotFound}
	r := newServer(t, server.WithRegenerator(stub))
	token, _ := signupAs(t, r, "regen-missing@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "summary"}, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown story_id, got %d: %v", w.Code, body)
	}
}

func TestRegenerate_CandidateExpired_410(t *testing.T) {
	truncate(t)
	stub := &stubRegenerator{storyErr: curation.ErrCandidateExpired}
	r := newServer(t, server.WithRegenerator(stub))
	token, _ := signupAs(t, r, "regen-expired@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/stories/"+storyID.String()+"/regenerate",
		map[string]any{"type": "summary"}, token)
	if w.Code != http.StatusGone {
		t.Fatalf("want 410 for expired candidate, got %d: %v", w.Code, body)
	}
}

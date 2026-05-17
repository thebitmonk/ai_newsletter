package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/issues"
)

// seedDraftedAt creates a drafted Issue at a specific scheduled_at so we can
// test the 60s approval freeze window precisely.
func seedDraftedAt(t *testing.T, pubID uuid.UUID, scheduledAt time.Time) uuid.UUID {
	t.Helper()
	store := issues.NewStore(testPool)
	iss, _, err := store.CreatePlanned(context.Background(), pubID, scheduledAt)
	if err != nil {
		t.Fatalf("seed planned: %v", err)
	}
	if _, err := store.ApplyTransition(context.Background(), iss.ID, issues.EventCurationStart, issues.TransitionUpdate{}); err != nil {
		t.Fatalf("planned->curating: %v", err)
	}
	subj := "x"; title := "y"; cover := "https://cdn/x.png"
	doc := json.RawMessage(`{"type":"doc","content":[{"type":"cover"}]}`)
	if _, err := store.ApplyTransition(context.Background(), iss.ID, issues.EventCurationOK, issues.TransitionUpdate{
		Subject: &subj, Title: &title, CoverURL: &cover, BodyDoc: doc,
	}); err != nil {
		t.Fatalf("curating->drafted: %v", err)
	}
	return iss.ID
}

func TestApprove_HappyPath(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "approve-happy@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	// scheduled well outside the freeze window
	issueID := seedDraftedAt(t, parseUUID(t, pubID), time.Now().Add(24*time.Hour))

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/approve", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["state"] != "approved" {
		t.Errorf("state: want approved, got %v", body["state"])
	}
}

func TestApprove_WithinFreezeWindow_409(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "approve-freeze@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	// scheduled 30s out — inside the 60s freeze window
	issueID := seedDraftedAt(t, parseUUID(t, pubID), time.Now().Add(30*time.Second))

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/approve", nil, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409 inside freeze window, got %d: %v", w.Code, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["code"] != "approval_window_closed" {
		t.Errorf("want approval_window_closed code, got %v", errBody)
	}
}

func TestApprove_CrossAccount_404(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, _ := signupAs(t, r, "approve-a@example.com")
	tokenB, _ := signupAs(t, r, "approve-b@example.com")
	pubA := makePubWithBrief(t, r, tokenA, "A", "brief", 1, 5)
	issueID := seedDraftedAt(t, parseUUID(t, pubA), time.Now().Add(24*time.Hour))

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/approve", nil, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-account approve: want 404, got %d", w.Code)
	}
}

func TestApprove_AlreadyApproved_409(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "approve-twice@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	issueID := seedDraftedAt(t, parseUUID(t, pubID), time.Now().Add(24*time.Hour))

	// First approve succeeds.
	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/approve", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("first approve: want 200, got %d", w.Code)
	}
	// Second approve refuses with wrong_state (state machine rejects
	// EventApprove from approved).
	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/issues/"+issueID.String()+"/approve", nil, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("second approve: want 409, got %d: %v", w.Code, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["code"] != "wrong_state" {
		t.Errorf("want wrong_state code, got %v", errBody)
	}
}

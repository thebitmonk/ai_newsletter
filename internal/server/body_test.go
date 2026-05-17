package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/server"
)

func TestUpdateBody_HappyPath(t *testing.T) {
	truncate(t)
	r := newServer(t, server.WithRegenerator(&stubRegenerator{}))
	token, _ := signupAs(t, r, "body-happy@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	storyID := uuid.New()
	issueID := seedDrafted(t, parseUUID(t, pubID), storyID)

	newDoc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "cover", "attrs": map[string]any{"block": "cover", "src": "https://cdn/x.png"}},
			map[string]any{"type": "intro", "attrs": map[string]any{"block": "intro"}, "content": []any{
				map[string]any{"type": "paragraph", "content": []any{
					map[string]any{"type": "text", "text": "Welcome back."},
				}},
			}},
		},
	}
	w, body := doJSON(t, r, http.MethodPut,
		"/api/v1/issues/"+issueID.String()+"/body",
		map[string]any{
			"subject":  "Edited subject",
			"title":    "Edited title",
			"body_doc": newDoc,
		}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["subject"] != "Edited subject" {
		t.Errorf("subject didn't persist: %v", body["subject"])
	}
}

func TestUpdateBody_WrongState_409(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "body-state@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	pubUUID := parseUUID(t, pubID)
	// Planned issue — body edits should be refused.
	issueID := seedPlanned(t, pubUUID, time.Now().Add(time.Hour))

	w, body := doJSON(t, r, http.MethodPut,
		"/api/v1/issues/"+issueID.String()+"/body",
		map[string]any{
			"subject":  "x",
			"title":    "y",
			"body_doc": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "cover"}}},
		}, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409 from planned-state body PUT, got %d: %v", w.Code, body)
	}
}

func TestUpdateBody_CrossAccount_404(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, _ := signupAs(t, r, "body-a@example.com")
	tokenB, _ := signupAs(t, r, "body-b@example.com")
	pubA := makePubWithBrief(t, r, tokenA, "A", "brief", 1, 5)
	issueID := seedDrafted(t, parseUUID(t, pubA), uuid.New())

	w, _ := doJSON(t, r, http.MethodPut,
		"/api/v1/issues/"+issueID.String()+"/body",
		map[string]any{
			"subject":  "stolen",
			"title":    "x",
			"body_doc": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "cover"}}},
		}, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-account PUT body: want 404, got %d", w.Code)
	}
}

func TestUpdateBody_InvalidBodyDoc_400(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "body-bad@example.com")
	pubID := makePubWithBrief(t, r, token, "P", "brief", 1, 5)
	issueID := seedDrafted(t, parseUUID(t, pubID), uuid.New())

	cases := []map[string]any{
		{"subject": "x", "title": "y", "body_doc": map[string]any{"type": "not-doc", "content": []any{1}}},
		{"subject": "x", "title": "y", "body_doc": map[string]any{"type": "doc", "content": []any{}}},
	}
	for i, payload := range cases {
		w, _ := doJSON(t, r, http.MethodPut,
			"/api/v1/issues/"+issueID.String()+"/body", payload, token)
		if w.Code != http.StatusBadRequest {
			t.Errorf("case %d: want 400, got %d", i, w.Code)
		}
	}
}

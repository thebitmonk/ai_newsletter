package server_test

import (
	"net/http"
	"testing"

)

func TestPublications_Create_HappyPath(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, accountID := signupAs(t, r, "pub-create@example.com")

	w, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":          "AI Weekly",
		"brief":         "Weekly LLM digest for engineers.",
		"timezone":      "America/New_York",
		"cadence_rule":  "FREQ=WEEKLY;BYDAY=MO;BYHOUR=9;BYMINUTE=0;BYSECOND=0",
	}, token)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %v", w.Code, body)
	}
	if body["account_id"] != accountID {
		t.Fatalf("account_id mismatch: %v vs %v", body["account_id"], accountID)
	}
	if body["name"] != "AI Weekly" {
		t.Fatalf("name mismatch: %v", body["name"])
	}
	if body["curation_lead_time"] != "24h0m0s" {
		t.Fatalf("default lead time mismatch: %v", body["curation_lead_time"])
	}
	if body["intro_enabled"] != true {
		t.Fatalf("intro_enabled default should be true: %v", body["intro_enabled"])
	}
}

func TestPublications_Create_BadTimezone(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-tz@example.com")

	w, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":     "Bad TZ",
		"timezone": "NotARealTimezone/Foo",
	}, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	if errBody, _ := body["error"].(map[string]any); errBody["code"] != "invalid_timezone" {
		t.Fatalf("want invalid_timezone code, got %v", errBody)
	}
}

func TestPublications_Create_BadRRULE(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-rrule@example.com")

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":         "Bad RRULE",
		"timezone":     "UTC",
		"cadence_rule": "NOT A REAL RRULE",
	}, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestPublications_Create_BadDuration(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-dur@example.com")

	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name":               "Bad duration",
		"timezone":           "UTC",
		"curation_lead_time": "garbage",
	}, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestPublications_Get_Found(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-get@example.com")

	_, created := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": "Get Me", "timezone": "UTC",
	}, token)
	id := created["id"].(string)

	w, body := doJSON(t, r, http.MethodGet, "/api/v1/publications/"+id, nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body["id"] != id {
		t.Fatalf("id mismatch: %v", body["id"])
	}
}

func TestPublications_Get_NotFound(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-404@example.com")
	w, _ := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/00000000-0000-0000-0000-000000000000", nil, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestPublications_Get_CrossAccount_404(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, _ := signupAs(t, r, "a@example.com")
	tokenB, _ := signupAs(t, r, "b@example.com")

	_, created := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": "A's pub", "timezone": "UTC",
	}, tokenA)
	id := created["id"].(string)

	w, _ := doJSON(t, r, http.MethodGet, "/api/v1/publications/"+id, nil, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-account read should 404, got %d", w.Code)
	}
}

func TestPublications_Update_HappyPath(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-upd@example.com")

	_, created := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": "Old name", "timezone": "UTC",
	}, token)
	id := created["id"].(string)

	w, body := doJSON(t, r, http.MethodPatch, "/api/v1/publications/"+id, map[string]any{
		"name":          "New name",
		"brief":         "Updated brief",
		"intro_enabled": false,
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["name"] != "New name" || body["brief"] != "Updated brief" || body["intro_enabled"] != false {
		t.Fatalf("update did not stick: %v", body)
	}
}

func TestPublications_Update_UnsetCadenceRule(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-unset@example.com")

	rule := "FREQ=WEEKLY;BYDAY=MO"
	_, created := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": "P", "timezone": "UTC", "cadence_rule": rule,
	}, token)
	id := created["id"].(string)

	w, body := doJSON(t, r, http.MethodPatch, "/api/v1/publications/"+id, map[string]any{
		"unset_cadence_rule": true,
	}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body["cadence_rule"] != nil {
		t.Fatalf("cadence_rule should be null after unset, got %v", body["cadence_rule"])
	}
}

func TestPublications_Update_CrossAccount_404(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, _ := signupAs(t, r, "uupd-a@example.com")
	tokenB, _ := signupAs(t, r, "uupd-b@example.com")

	_, created := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": "A's pub", "timezone": "UTC",
	}, tokenA)
	id := created["id"].(string)

	w, _ := doJSON(t, r, http.MethodPatch, "/api/v1/publications/"+id,
		map[string]any{"name": "stolen"}, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-account update should 404, got %d", w.Code)
	}
}

func TestPublications_Delete(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, _ := signupAs(t, r, "pub-del@example.com")

	_, created := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": "Doomed", "timezone": "UTC",
	}, token)
	id := created["id"].(string)

	w, _ := doJSON(t, r, http.MethodDelete, "/api/v1/publications/"+id, nil, token)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}

	// Subsequent get is 404.
	w, _ = doJSON(t, r, http.MethodGet, "/api/v1/publications/"+id, nil, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete should 404, got %d", w.Code)
	}
}

func TestPublications_List_PaginationAndAccountScope(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, _ := signupAs(t, r, "list-a@example.com")
	tokenB, _ := signupAs(t, r, "list-b@example.com")

	// Create 5 for A, 2 for B.
	for range 5 {
		_, _ = doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
			"name": "A-pub", "timezone": "UTC",
		}, tokenA)
	}
	for range 2 {
		_, _ = doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
			"name": "B-pub", "timezone": "UTC",
		}, tokenB)
	}

	// First page of 2 for A.
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/publications?limit=2", nil, tokenA)
	if w.Code != http.StatusOK {
		t.Fatalf("list page 1: %d", w.Code)
	}
	page1, _ := body["items"].([]any)
	if len(page1) != 2 {
		t.Fatalf("page 1 expected 2 items, got %d", len(page1))
	}
	nextCursor, _ := body["next_cursor"].(string)
	if nextCursor == "" {
		t.Fatal("expected next_cursor on first page")
	}

	// Second page.
	w, body = doJSON(t, r, http.MethodGet, "/api/v1/publications?limit=2&cursor="+nextCursor, nil, tokenA)
	page2, _ := body["items"].([]any)
	if len(page2) != 2 {
		t.Fatalf("page 2 expected 2 items, got %d", len(page2))
	}

	// Third page (1 item left, no more cursor).
	nextCursor, _ = body["next_cursor"].(string)
	w, body = doJSON(t, r, http.MethodGet, "/api/v1/publications?limit=2&cursor="+nextCursor, nil, tokenA)
	page3, _ := body["items"].([]any)
	if len(page3) != 1 {
		t.Fatalf("page 3 expected 1 item, got %d", len(page3))
	}
	if body["next_cursor"] != nil {
		t.Fatalf("expected no next_cursor at end, got %v", body["next_cursor"])
	}

	// B's list excludes A's pubs.
	w, body = doJSON(t, r, http.MethodGet, "/api/v1/publications", nil, tokenB)
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("B should see exactly 2 publications, got %d", len(items))
	}
}

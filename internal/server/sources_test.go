package server_test

import (
	"net/http"
	"testing"

	"github.com/thebitmonk/ai_newsletter/internal/server"
)

// makePub is a test helper that creates a publication and returns its id.
func makePub(t *testing.T, r http.Handler, token, name string) string {
	t.Helper()
	_, body := doJSON(t, r, http.MethodPost, "/api/v1/publications", map[string]any{
		"name": name, "timezone": "UTC",
	}, token)
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("makePub failed: %v", body)
	}
	return id
}

func TestSources_Create_HappyPath(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-create@example.com")
	pubID := makePub(t, r, token, "P")

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources", map[string]any{
			"type":       "rss",
			"identifier": "https://example.com/feed.xml",
		}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %v", w.Code, body)
	}
	if body["type"] != "rss" || body["identifier"] != "https://example.com/feed.xml" {
		t.Fatalf("unexpected body: %v", body)
	}
	if body["poll_interval"] != "1h0m0s" {
		t.Fatalf("expected default rss poll_interval 1h, got %v", body["poll_interval"])
	}
}

func TestSources_Create_DefaultIntervalsPerType(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-defaults@example.com")
	pubID := makePub(t, r, token, "P")

	cases := []struct {
		typ, ident, want string
	}{
		{"rss", "https://a.com/feed", "1h0m0s"},
		{"youtube_channel", "UCAAAAAAAAAAAAAAAAAAAAAA", "6h0m0s"},
		{"x_handle", "@karpathy", "30m0s"},
		{"substack", "https://stratechery.com/feed", "1h0m0s"},
		{"web", "https://anthropic.com/news", "4h0m0s"},
	}
	for _, tc := range cases {
		w, body := doJSON(t, r, http.MethodPost,
			"/api/v1/publications/"+pubID+"/sources", map[string]any{
				"type": tc.typ, "identifier": tc.ident,
			}, token)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: want 201 got %d: %v", tc.typ, w.Code, body)
		}
		if body["poll_interval"] != tc.want {
			t.Fatalf("%s default interval: want %s got %v", tc.typ, tc.want, body["poll_interval"])
		}
	}
}

func TestSources_Create_NormalisesXHandle(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-x@example.com")
	pubID := makePub(t, r, token, "P")

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources", map[string]any{
			"type": "x_handle", "identifier": "@karpathy",
		}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", w.Code)
	}
	if body["identifier"] != "karpathy" {
		t.Fatalf("@ should be stripped: got %v", body["identifier"])
	}
}

func TestSources_Create_BadType(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-bt@example.com")
	pubID := makePub(t, r, token, "P")

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources", map[string]any{
			"type": "not_a_type", "identifier": "https://x.com",
		}, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestSources_Create_BadIdentifierPerType(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-bi@example.com")
	pubID := makePub(t, r, token, "P")

	cases := []struct {
		typ, ident string
	}{
		{"rss", "not-a-url"},
		{"web", "ftp://example.com/feed"},
		{"youtube_channel", "not-a-channel-id"},
		{"x_handle", "this_handle_is_way_too_long_to_be_valid"},
	}
	for _, tc := range cases {
		w, _ := doJSON(t, r, http.MethodPost,
			"/api/v1/publications/"+pubID+"/sources", map[string]any{
				"type": tc.typ, "identifier": tc.ident,
			}, token)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s with %q: want 400 got %d", tc.typ, tc.ident, w.Code)
		}
	}
}

func TestSources_Create_DuplicateConflict(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-dup@example.com")
	pubID := makePub(t, r, token, "P")

	_, _ = doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources", map[string]any{
			"type": "rss", "identifier": "https://example.com/feed",
		}, token)

	w, body := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources", map[string]any{
			"type": "rss", "identifier": "https://example.com/feed",
		}, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %v", w.Code, body)
	}
}

func TestSources_Create_CrossAccount_PubNotFound(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	tokenA, _ := signupAs(t, r, "sa@example.com")
	tokenB, _ := signupAs(t, r, "sb@example.com")
	pubA := makePub(t, r, tokenA, "A pub")

	w, _ := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubA+"/sources", map[string]any{
			"type": "rss", "identifier": "https://x.com/feed",
		}, tokenB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("B should 404 creating under A's pub, got %d", w.Code)
	}
}

func TestSources_List_ScopedToPublication(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-list@example.com")
	pub1 := makePub(t, r, token, "P1")
	pub2 := makePub(t, r, token, "P2")

	_, _ = doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pub1+"/sources", map[string]any{
			"type": "rss", "identifier": "https://1.com/feed",
		}, token)
	_, _ = doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pub2+"/sources", map[string]any{
			"type": "rss", "identifier": "https://2.com/feed",
		}, token)

	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pub1+"/sources", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 source under pub1, got %d", len(items))
	}
}

func TestSources_Update_HappyAndCrossAccount(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-upd@example.com")
	other, _ := signupAs(t, r, "src-upd-other@example.com")
	pubID := makePub(t, r, token, "P")

	_, created := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources", map[string]any{
			"type": "rss", "identifier": "https://orig.com/feed",
		}, token)
	srcID := created["id"].(string)

	w, body := doJSON(t, r, http.MethodPatch,
		"/api/v1/publications/"+pubID+"/sources/"+srcID, map[string]any{
			"identifier":    "https://updated.com/feed",
			"poll_interval": "2h",
			"enabled":       false,
		}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["identifier"] != "https://updated.com/feed" || body["enabled"] != false || body["poll_interval"] != "2h0m0s" {
		t.Fatalf("update did not stick: %v", body)
	}

	w, _ = doJSON(t, r, http.MethodPatch,
		"/api/v1/publications/"+pubID+"/sources/"+srcID, map[string]any{
			"enabled": true,
		}, other)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-account update should 404, got %d", w.Code)
	}
}

func TestSources_Delete(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	token, _ := signupAs(t, r, "src-del@example.com")
	pubID := makePub(t, r, token, "P")

	_, created := doJSON(t, r, http.MethodPost,
		"/api/v1/publications/"+pubID+"/sources", map[string]any{
			"type": "rss", "identifier": "https://gone.com/feed",
		}, token)
	srcID := created["id"].(string)

	w, _ := doJSON(t, r, http.MethodDelete,
		"/api/v1/publications/"+pubID+"/sources/"+srcID, nil, token)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}

	w, _ = doJSON(t, r, http.MethodGet,
		"/api/v1/publications/"+pubID+"/sources/"+srcID, nil, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete should 404, got %d", w.Code)
	}
}

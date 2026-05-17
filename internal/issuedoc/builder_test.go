package issuedoc_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/issuedoc"
)

func mustAssemble(t *testing.T, a issuedoc.IssueAssembly) map[string]any {
	t.Helper()
	raw, err := issuedoc.Assemble(a)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

func sampleStories(n int) []issuedoc.Story {
	out := make([]issuedoc.Story, n)
	for i := range out {
		out[i] = issuedoc.Story{
			ID:       uuid.New(),
			URL:      "https://example.com/" + string(rune('a'+i)),
			Headline: "Headline " + string(rune('A'+i)),
			Body:     "Body for story " + string(rune('A'+i)),
		}
	}
	return out
}

func TestAssemble_HappyPath(t *testing.T) {
	stories := sampleStories(3)
	doc := mustAssemble(t, issuedoc.IssueAssembly{
		Subject:  "Weekly AI Roundup",
		Title:    "Five things this week",
		CoverURL: "https://blobs/cover.png",
		Intro:    "Welcome back to AI Weekly.",
		Stories:  stories,
	})

	if doc["type"] != "doc" {
		t.Fatalf("root type: %v", doc["type"])
	}
	attrs := doc["attrs"].(map[string]any)
	if int(attrs["version"].(float64)) != issuedoc.SchemaVersion {
		t.Errorf("version mismatch: %v", attrs["version"])
	}
	if attrs["subject"] != "Weekly AI Roundup" {
		t.Errorf("subject not in attrs")
	}

	content := doc["content"].([]any)
	// expect: cover, intro, then N story nodes
	if len(content) != 2+len(stories) {
		t.Fatalf("want %d content nodes, got %d", 2+len(stories), len(content))
	}
	if content[0].(map[string]any)["type"] != "cover" {
		t.Errorf("first node should be cover")
	}
	if content[1].(map[string]any)["type"] != "intro" {
		t.Errorf("second node should be intro")
	}
	for i, s := range stories {
		node := content[2+i].(map[string]any)
		if node["type"] != "story" {
			t.Errorf("story %d type: %v", i, node["type"])
		}
		nodeAttrs := node["attrs"].(map[string]any)
		if nodeAttrs["storyId"] != s.ID.String() {
			t.Errorf("story %d storyId attr missing", i)
		}
		if nodeAttrs["block"] != "story" {
			t.Errorf("story %d block attr missing", i)
		}
	}
}

func TestAssemble_OmitsIntroWhenEmpty(t *testing.T) {
	doc := mustAssemble(t, issuedoc.IssueAssembly{
		Subject:  "x",
		Title:    "x",
		CoverURL: "https://blobs/x.png",
		Intro:    "",
		Stories:  sampleStories(1),
	})
	content := doc["content"].([]any)
	if len(content) != 2 { // cover + 1 story, no intro
		t.Fatalf("want 2 content nodes, got %d", len(content))
	}
	for _, n := range content {
		if n.(map[string]any)["type"] == "intro" {
			t.Fatal("intro should be absent when Intro is empty")
		}
	}
}

func TestAssemble_RequiresFields(t *testing.T) {
	base := issuedoc.IssueAssembly{
		Subject:  "s",
		Title:    "t",
		CoverURL: "https://blobs/x.png",
		Stories:  sampleStories(1),
	}
	cases := []struct {
		name string
		mut  func(*issuedoc.IssueAssembly)
	}{
		{"no subject", func(a *issuedoc.IssueAssembly) { a.Subject = "" }},
		{"no title", func(a *issuedoc.IssueAssembly) { a.Title = "" }},
		{"no cover", func(a *issuedoc.IssueAssembly) { a.CoverURL = "" }},
		{"no stories", func(a *issuedoc.IssueAssembly) { a.Stories = nil }},
		{"story missing id", func(a *issuedoc.IssueAssembly) {
			a.Stories = []issuedoc.Story{{ID: uuid.Nil, Headline: "h", Body: "b"}}
		}},
	}
	for _, tc := range cases {
		a := base
		tc.mut(&a)
		if _, err := issuedoc.Assemble(a); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}

func TestAssemble_CoverAttrsCarrySrc(t *testing.T) {
	doc := mustAssemble(t, issuedoc.IssueAssembly{
		Subject:  "s",
		Title:    "T",
		CoverURL: "https://cdn.example/cover-abc.png",
		Stories:  sampleStories(1),
	})
	cover := doc["content"].([]any)[0].(map[string]any)
	attrs := cover["attrs"].(map[string]any)
	if attrs["src"] != "https://cdn.example/cover-abc.png" {
		t.Errorf("cover src: %v", attrs["src"])
	}
	if !strings.Contains(attrs["alt"].(string), "T") {
		t.Errorf("cover alt should default to title")
	}
}

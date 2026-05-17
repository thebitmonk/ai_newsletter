// Package issuedoc assembles a Publication-ready ProseMirror document for an
// Issue per ADR-0008. The doc is the source of truth for the Issue body; the
// structured columns (subject, title, cover_url) are denormalised for fast
// queries but their values are also embedded as node attrs so the editor can
// roundtrip without losing them.
//
// The schema below is intentionally narrow at v1: cover, intro, story. The
// sponsor block from the Issue anatomy lands when the sponsor feature does.
package issuedoc

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// SchemaVersion is the ProseMirror doc schema version we currently emit.
// Persisted in issues.body_doc_version and validated on read.
const SchemaVersion = 1

// IssueAssembly is the input to Assemble.
type IssueAssembly struct {
	Subject  string
	Title    string
	CoverURL string
	Intro    string // empty -> no intro node
	Stories  []Story
}

// Story is one curated item to embed in the doc.
type Story struct {
	ID       uuid.UUID // becomes data-story-id; used by regen lookups
	URL      string    // source URL (e.g. video, post)
	Headline string
	Body     string
}

// Assemble builds the ProseMirror doc.
func Assemble(a IssueAssembly) (json.RawMessage, error) {
	if a.Subject == "" {
		return nil, errors.New("issuedoc: subject is required")
	}
	if a.Title == "" {
		return nil, errors.New("issuedoc: title is required")
	}
	if a.CoverURL == "" {
		return nil, errors.New("issuedoc: coverURL is required")
	}
	if len(a.Stories) == 0 {
		return nil, errors.New("issuedoc: at least one story is required")
	}

	content := []map[string]any{
		coverNode(a.CoverURL, a.Title),
	}
	if a.Intro != "" {
		content = append(content, introNode(a.Intro))
	}
	for _, s := range a.Stories {
		if s.ID == uuid.Nil {
			return nil, fmt.Errorf("issuedoc: story id is required for headline %q", s.Headline)
		}
		content = append(content, storyNode(s))
	}

	doc := map[string]any{
		"type": "doc",
		"attrs": map[string]any{
			"version": SchemaVersion,
			"subject": a.Subject,
			"title":   a.Title,
		},
		"content": content,
	}
	return json.Marshal(doc)
}

func coverNode(src, alt string) map[string]any {
	return map[string]any{
		"type": "cover",
		"attrs": map[string]any{
			"src":   src,
			"alt":   alt,
			"block": "cover", // becomes data-block="cover" on render
		},
	}
}

func introNode(text string) map[string]any {
	return map[string]any{
		"type": "intro",
		"attrs": map[string]any{
			"block": "intro",
		},
		"content": []map[string]any{
			{
				"type":    "paragraph",
				"content": []map[string]any{{"type": "text", "text": text}},
			},
		},
	}
}

func storyNode(s Story) map[string]any {
	return map[string]any{
		"type": "story",
		"attrs": map[string]any{
			"block":     "story",
			"storyId":   s.ID.String(),
			"sourceUrl": s.URL,
		},
		"content": []map[string]any{
			{
				"type": "paragraph",
				"attrs": map[string]any{"role": "headline"},
				"content": []map[string]any{
					{"type": "text", "text": s.Headline},
				},
			},
			{
				"type": "paragraph",
				"attrs": map[string]any{"role": "body"},
				"content": []map[string]any{
					{"type": "text", "text": s.Body},
				},
			},
		},
	}
}

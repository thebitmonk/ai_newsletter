package curation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/imagegen"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
)

// ErrStoryNodeNotFound is returned when no story node matches storyID in the
// Issue's body_doc.
var ErrStoryNodeNotFound = errors.New("story node not found in body doc")

// ErrCandidateExpired is returned when the originating Candidate for a Story
// has aged out of the pool, so we have no source content to re-summarise.
var ErrCandidateExpired = errors.New("originating candidate expired from pool")

// ErrWrongState is returned when regenerate is called on an Issue not in
// drafted or approved.
var ErrWrongState = errors.New("regenerate requires drafted or approved state")

// RegenerateStorySummary re-runs the Summarizer for one Story in an Issue,
// replacing the targeted node's headline + body paragraphs while preserving
// the node's stable storyId / sourceUrl / block attrs and leaving every
// other node untouched.
//
// Issue must be in drafted or approved state. Cross-account check happens at
// the HTTP handler layer via GetForAccount.
func (w *Worker) RegenerateStorySummary(ctx context.Context, iss *issues.Issue, storyID uuid.UUID) (*issues.Issue, error) {
	if iss.State != issues.StateDrafted && iss.State != issues.StateApproved {
		return nil, ErrWrongState
	}
	pub, err := w.loadPublication(ctx, iss.PublicationID)
	if err != nil {
		return nil, fmt.Errorf("load publication: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(iss.BodyDoc, &doc); err != nil {
		return nil, fmt.Errorf("parse body_doc: %w", err)
	}
	node, sourceURL, err := findStoryNode(doc, storyID.String())
	if err != nil {
		return nil, err
	}

	cand, err := w.candidates.GetByURL(ctx, iss.PublicationID, sourceURL)
	if errors.Is(err, candidates.ErrNotFound) {
		return nil, ErrCandidateExpired
	}
	if err != nil {
		return nil, fmt.Errorf("lookup candidate: %w", err)
	}

	summary, err := w.summarizer.Summarize(ctx, pub.Brief, *cand)
	if err != nil {
		return nil, fmt.Errorf("summarize: %w", err)
	}

	if err := updateStoryNodeContent(node, summary.Headline, summary.Body); err != nil {
		return nil, fmt.Errorf("update node: %w", err)
	}

	newDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("re-marshal doc: %w", err)
	}

	if err := w.persistBodyDoc(ctx, iss.ID, newDoc, nil); err != nil {
		return nil, err
	}
	return w.issues.Get(ctx, iss.ID)
}

// RegenerateCover re-runs the ImageGenerator and updates both the cover_url
// column and the doc's cover node src attr per ADR-0008's sync invariant.
func (w *Worker) RegenerateCover(ctx context.Context, iss *issues.Issue) (*issues.Issue, error) {
	if iss.State != issues.StateDrafted && iss.State != issues.StateApproved {
		return nil, ErrWrongState
	}
	pub, err := w.loadPublication(ctx, iss.PublicationID)
	if err != nil {
		return nil, fmt.Errorf("load publication: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(iss.BodyDoc, &doc); err != nil {
		return nil, fmt.Errorf("parse body_doc: %w", err)
	}

	// Build prompt hints from the Issue's title + Story headlines.
	hints := imagegen.PromptHints{}
	if iss.Title != nil {
		hints.IssueTitle = *iss.Title
	}
	for _, hl := range extractStoryHeadlines(doc) {
		hints.StorySummaries = append(hints.StorySummaries, hl)
	}

	cover, err := w.images.Generate(ctx, pub.Brief, hints)
	if err != nil {
		return nil, fmt.Errorf("image generate: %w", err)
	}

	if err := updateCoverNodeSrc(doc, cover.URL); err != nil {
		return nil, fmt.Errorf("update cover node: %w", err)
	}
	newDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("re-marshal doc: %w", err)
	}

	if err := w.persistBodyDoc(ctx, iss.ID, newDoc, &cover.URL); err != nil {
		return nil, err
	}
	return w.issues.Get(ctx, iss.ID)
}

// persistBodyDoc updates issues.body_doc (and cover_url when supplied). No
// state transition — regenerate is a content mutation, not a lifecycle event.
func (w *Worker) persistBodyDoc(ctx context.Context, issueID uuid.UUID, doc json.RawMessage, coverURL *string) error {
	_, err := w.pool.Exec(ctx, `
		update issues
		set body_doc   = $2::jsonb,
		    cover_url  = coalesce($3, cover_url),
		    updated_at = now()
		where id = $1
	`, issueID, doc, coverURL)
	return err
}

// findStoryNode walks doc.content looking for a story node with the given
// storyId attr. Returns the node map (for in-place mutation) and its sourceUrl.
func findStoryNode(doc map[string]any, storyID string) (map[string]any, string, error) {
	content, ok := doc["content"].([]any)
	if !ok {
		return nil, "", ErrStoryNodeNotFound
	}
	for _, raw := range content {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := node["type"].(string); t != "story" {
			continue
		}
		attrs, _ := node["attrs"].(map[string]any)
		if attrs == nil {
			continue
		}
		if id, _ := attrs["storyId"].(string); id == storyID {
			src, _ := attrs["sourceUrl"].(string)
			return node, src, nil
		}
	}
	return nil, "", ErrStoryNodeNotFound
}

// updateStoryNodeContent replaces the two paragraphs (headline + body) in a
// story node while preserving node-level attrs.
func updateStoryNodeContent(node map[string]any, headline, body string) error {
	node["content"] = []any{
		map[string]any{
			"type":  "paragraph",
			"attrs": map[string]any{"role": "headline"},
			"content": []any{
				map[string]any{"type": "text", "text": headline},
			},
		},
		map[string]any{
			"type":  "paragraph",
			"attrs": map[string]any{"role": "body"},
			"content": []any{
				map[string]any{"type": "text", "text": body},
			},
		},
	}
	return nil
}

// updateCoverNodeSrc replaces the src attr on the doc's cover node, leaving
// the rest of the node (including its alt + block attrs) untouched.
func updateCoverNodeSrc(doc map[string]any, src string) error {
	content, ok := doc["content"].([]any)
	if !ok {
		return errors.New("doc has no content")
	}
	for _, raw := range content {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := node["type"].(string); t != "cover" {
			continue
		}
		attrs, _ := node["attrs"].(map[string]any)
		if attrs == nil {
			attrs = map[string]any{}
		}
		attrs["src"] = src
		node["attrs"] = attrs
		return nil
	}
	return errors.New("cover node not found in body doc")
}

// extractStoryHeadlines returns the headline text from each story node, in doc
// order. Used to seed ImageGenerator prompt hints.
func extractStoryHeadlines(doc map[string]any) []string {
	content, _ := doc["content"].([]any)
	out := []string{}
	for _, raw := range content {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := node["type"].(string); t != "story" {
			continue
		}
		paras, _ := node["content"].([]any)
		for _, p := range paras {
			par, ok := p.(map[string]any)
			if !ok {
				continue
			}
			attrs, _ := par["attrs"].(map[string]any)
			role, _ := attrs["role"].(string)
			if role != "headline" {
				continue
			}
			texts, _ := par["content"].([]any)
			for _, t := range texts {
				tm, ok := t.(map[string]any)
				if !ok {
					continue
				}
				if text, _ := tm["text"].(string); text != "" {
					out = append(out, text)
				}
			}
		}
	}
	return out
}

// Package llmclient implements the LLMRanker and Summarizer interfaces
// against OpenAI's Chat Completions API. The model is configurable via
// OPENAI_MODEL (default gpt-5.4-mini per the project's curation-pipeline
// decision).
//
// The Brief from the Publication is injected verbatim into every call
// per ADR-0005 — see the prompt-building helpers below.
package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
)

const DefaultModel = "gpt-5.4-mini"

// LLMRanker scores Candidates against a Brief and returns the top N.
type LLMRanker interface {
	Rank(ctx context.Context, brief string, cands []candidates.Candidate, n int) ([]ScoredCandidate, error)
}

// Summarizer produces per-Story summaries and Issue-level cover-copy text
// (Subject, Title, Intro). The Brief is the editorial spine for all of them.
type Summarizer interface {
	Summarize(ctx context.Context, brief string, c candidates.Candidate) (Summary, error)
	Headline(ctx context.Context, brief string, summaries []Summary) (Headline, error)
	Intro(ctx context.Context, brief string, summaries []Summary) (string, error)
}

// ScoredCandidate is the output of a Rank call.
type ScoredCandidate struct {
	Candidate candidates.Candidate
	Score     float64
	Reason    string
}

// Summary is the per-Story output: a one-line headline and a longer body.
type Summary struct {
	Headline string
	Body     string
}

// Headline is the Issue-level Subject + Title pair.
type Headline struct {
	Subject string // for the email Subject: header, <= 50 chars
	Title   string // the in-Issue headline
}

// Client wraps the OpenAI client with the model id and the prompt logic.
type Client struct {
	openai openai.Client
	model  string
}

// NewFromEnv loads OPENAI_API_KEY (required) and OPENAI_MODEL (optional,
// default gpt-5.4-mini). Returns an error if OPENAI_API_KEY is missing.
func NewFromEnv() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("llmclient: OPENAI_API_KEY is required")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		openai: openai.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}, nil
}

// New constructs a Client with explicit args (mostly for tests).
func New(apiKey, model string) *Client {
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		openai: openai.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

// Rank scores cands against brief and returns the top n. The model responds
// in JSON with one entry per candidate; entries below the threshold (0.5)
// are dropped; the rest are sorted by score desc.
func (c *Client) Rank(ctx context.Context, brief string, cands []candidates.Candidate, n int) ([]ScoredCandidate, error) {
	if len(cands) == 0 {
		return nil, nil
	}

	candList := strings.Builder{}
	for i, cd := range cands {
		fmt.Fprintf(&candList, "%d. id=%s title=%q url=%s\n", i+1, cd.ID, cd.Title, cd.URL)
	}

	user := fmt.Sprintf(`PUBLICATION BRIEF:
%s

CANDIDATES TO SCORE:
%s

Respond with ONLY a JSON object of shape:
{"scored": [{"index": <1-based>, "score": <0..1>, "reason": "<one sentence>"}, ...]}

Include EVERY candidate. score >= 0.5 means worth including in the issue.`, brief, candList.String())

	resp, err := c.openai.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(rankerSystem),
			openai.UserMessage(user),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ranker: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("ranker: empty response")
	}

	var parsed struct {
		Scored []struct {
			Index  int     `json:"index"`
			Score  float64 `json:"score"`
			Reason string  `json:"reason"`
		} `json:"scored"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &parsed); err != nil {
		return nil, fmt.Errorf("ranker: parse: %w", err)
	}

	out := make([]ScoredCandidate, 0, len(parsed.Scored))
	for _, s := range parsed.Scored {
		if s.Index < 1 || s.Index > len(cands) {
			continue
		}
		out = append(out, ScoredCandidate{
			Candidate: cands[s.Index-1],
			Score:     s.Score,
			Reason:    s.Reason,
		})
	}
	// Sort score desc.
	sortDesc(out)
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// Summarize produces a Summary for one Candidate.
func (c *Client) Summarize(ctx context.Context, brief string, cd candidates.Candidate) (Summary, error) {
	user := fmt.Sprintf(`PUBLICATION BRIEF:
%s

CANDIDATE:
title: %s
url:   %s
raw_content (JSON, opaque per-platform): %s

Write a Story for this candidate in the Publication's voice. Output JSON ONLY:
{"headline": "<one-line headline 5-12 words>", "body": "<2-4 sentence summary>"}`,
		brief, cd.Title, cd.URL, string(cd.Raw))

	resp, err := c.openai.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(summarizerSystem),
			openai.UserMessage(user),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		},
	})
	if err != nil {
		return Summary{}, fmt.Errorf("summarizer: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Summary{}, errors.New("summarizer: empty response")
	}

	var parsed Summary
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &parsed); err != nil {
		return Summary{}, fmt.Errorf("summarizer: parse: %w", err)
	}
	return parsed, nil
}

// Headline produces the Issue-level Subject + Title.
func (c *Client) Headline(ctx context.Context, brief string, summaries []Summary) (Headline, error) {
	bullets := strings.Builder{}
	for _, s := range summaries {
		fmt.Fprintf(&bullets, "- %s\n", s.Headline)
	}

	user := fmt.Sprintf(`PUBLICATION BRIEF:
%s

ISSUE STORY HEADLINES:
%s

Produce a Subject line (<= 50 chars, inbox preview-friendly) and a Title
(in-issue headline, sharper, no length cap but keep it punchy). Output JSON:
{"subject": "<...>", "title": "<...>"}`, brief, bullets.String())

	resp, err := c.openai.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(summarizerSystem),
			openai.UserMessage(user),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		},
	})
	if err != nil {
		return Headline{}, fmt.Errorf("headline: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Headline{}, errors.New("headline: empty response")
	}
	var parsed Headline
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &parsed); err != nil {
		return Headline{}, fmt.Errorf("headline: parse: %w", err)
	}
	return parsed, nil
}

// Intro produces the optional intro paragraph for an Issue.
func (c *Client) Intro(ctx context.Context, brief string, summaries []Summary) (string, error) {
	bullets := strings.Builder{}
	for _, s := range summaries {
		fmt.Fprintf(&bullets, "- %s\n", s.Headline)
	}
	user := fmt.Sprintf(`PUBLICATION BRIEF:
%s

ISSUE STORY HEADLINES:
%s

Write a 1-2 sentence framing intro for this issue in the Publication's voice.
Plain text, no markdown.`, brief, bullets.String())

	resp, err := c.openai.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(summarizerSystem),
			openai.UserMessage(user),
		},
	})
	if err != nil {
		return "", fmt.Errorf("intro: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("intro: empty response")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

const (
	rankerSystem = `You are a newsletter editor. Score each candidate item against the publication's brief on a 0-1 scale:
- 1.0 = perfectly fits the brief
- 0.7 = relevant
- 0.5 = marginal, include only if higher-scored items are scarce
- 0.0 = irrelevant or off-brand
Be strict.`

	summarizerSystem = `You are a newsletter writer. Match the Publication's voice exactly as described in its brief. Be concise, factual, and link-aware. Do NOT invent details that are not in the source.`
)

// sortDesc sorts ScoredCandidates by score desc, stable.
func sortDesc(s []ScoredCandidate) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Score > s[j-1].Score; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

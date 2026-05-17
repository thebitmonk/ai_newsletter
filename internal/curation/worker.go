// Package curation drives a planned Issue through curating to drafted (or
// skipped/failed). It is the orchestrator that pulls Candidates, ranks them
// against the Publication Brief, summarises them, generates a cover image,
// assembles the ProseMirror body doc, and persists the result.
//
// Per ADR-0005 the Brief is injected into every LLM call. Per ADR-0006 the
// cover image is per-Issue, not per-Story. Per ADR-0008 the body is one
// ProseMirror doc with operational metadata duplicated as structured columns.
package curation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nsqio/go-nsq"

	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/imagegen"
	"github.com/thebitmonk/ai_newsletter/internal/issuedoc"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/llmclient"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
)

// StartTopic is the NSQ topic the curation worker consumes.
var StartTopic = nsqx.Topic("curation", "start")

// StartMessage is the JSON body of a curation.start message. Per ADR-0015
// only an issue id travels over the wire; the worker re-fetches state.
type StartMessage struct {
	IssueID string `json:"issue_id"`
}

// Worker orchestrates the curation pipeline.
type Worker struct {
	pool       *pgxpool.Pool
	issues     *issues.Store
	candidates *candidates.Store
	ranker     llmclient.LLMRanker
	summarizer llmclient.Summarizer
	images     imagegen.ImageGenerator

	// CandidateLookback bounds how far back ListActive looks. Defaults to 30d.
	CandidateLookback time.Duration

	// ScoreThreshold is the minimum LLMRanker score to include a Candidate.
	// Defaults to 0.5.
	ScoreThreshold float64
}

// NewWorker assembles a Worker. All dependencies must be non-nil except as
// documented (e.g. images may be nil if cover generation is disabled, but at
// v1 cover is required so images is required too).
func NewWorker(
	pool *pgxpool.Pool,
	is *issues.Store,
	cs *candidates.Store,
	r llmclient.LLMRanker,
	s llmclient.Summarizer,
	ig imagegen.ImageGenerator,
) *Worker {
	return &Worker{
		pool:              pool,
		issues:            is,
		candidates:        cs,
		ranker:            r,
		summarizer:        s,
		images:            ig,
		CandidateLookback: 30 * 24 * time.Hour,
		ScoreThreshold:    0.5,
	}
}

// HandleMessage is the NSQ handler. Always returns nil so NSQ does not
// requeue — failures are recorded in the Issue state.
func (w *Worker) HandleMessage(msg *nsq.Message) error {
	var m StartMessage
	if err := json.Unmarshal(msg.Body, &m); err != nil {
		log.Printf("curation: bad payload: %v", err)
		return nil
	}
	id, err := uuid.Parse(m.IssueID)
	if err != nil {
		log.Printf("curation: bad issue_id: %v", err)
		return nil
	}
	if err := w.Curate(context.Background(), id); err != nil {
		log.Printf("curation: %s: %v", id, err)
	}
	return nil
}

// publication is the subset of fields curation cares about. Loaded inline to
// avoid pulling the publications.Store into the curation deps.
type publication struct {
	ID                  uuid.UUID
	Brief               string
	StoriesPerIssueMin  int
	StoriesPerIssueMax  int
	IntroEnabled        bool
}

func (w *Worker) loadPublication(ctx context.Context, id uuid.UUID) (*publication, error) {
	var p publication
	err := w.pool.QueryRow(ctx, `
		select id, brief, stories_per_issue_min, stories_per_issue_max, intro_enabled
		from publications where id = $1
	`, id).Scan(&p.ID, &p.Brief, &p.StoriesPerIssueMin, &p.StoriesPerIssueMax, &p.IntroEnabled)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Curate runs the full pipeline for a single Issue. Public so dev/admin
// triggers and tests can invoke directly without NSQ.
func (w *Worker) Curate(ctx context.Context, issueID uuid.UUID) error {
	iss, err := w.issues.Get(ctx, issueID)
	if err != nil {
		return fmt.Errorf("load issue: %w", err)
	}
	if iss.State != issues.StatePlanned {
		// Idempotency guard — drop late re-deliveries silently.
		log.Printf("curation: issue %s state=%s, not planned, skipping", iss.ID, iss.State)
		return nil
	}

	pub, err := w.loadPublication(ctx, iss.PublicationID)
	if err != nil {
		return fmt.Errorf("load publication: %w", err)
	}

	if _, err := w.issues.ApplyTransition(ctx, iss.ID, issues.EventCurationStart, issues.TransitionUpdate{}); err != nil {
		return fmt.Errorf("transition planned->curating: %w", err)
	}

	if err := w.runPipeline(ctx, iss.ID, pub); err != nil {
		reason := err.Error()
		_, txErr := w.issues.ApplyTransition(ctx, iss.ID, issues.EventCurationError, issues.TransitionUpdate{
			FailedReason: &reason,
		})
		if txErr != nil {
			log.Printf("curation: %s failed-transition: %v", iss.ID, txErr)
		}
		return err
	}
	return nil
}

// runPipeline does the work between curating and drafted. Returns nil on
// success (the drafted transition has been applied) or an error that the
// caller turns into a failed transition.
func (w *Worker) runPipeline(ctx context.Context, issueID uuid.UUID, pub *publication) error {
	cands, err := w.candidates.ListActive(ctx, pub.ID, time.Now().Add(-w.CandidateLookback))
	if err != nil {
		return fmt.Errorf("list candidates: %w", err)
	}
	if len(cands) == 0 {
		_, err := w.issues.ApplyTransition(ctx, issueID, issues.EventNoCandidates, issues.TransitionUpdate{})
		if err != nil {
			return fmt.Errorf("transition curating->skipped: %w", err)
		}
		return nil
	}

	ranked, err := w.ranker.Rank(ctx, pub.Brief, cands, pub.StoriesPerIssueMax)
	if err != nil {
		return fmt.Errorf("rank: %w", err)
	}

	selected := make([]llmclient.ScoredCandidate, 0, len(ranked))
	for _, r := range ranked {
		if r.Score >= w.ScoreThreshold {
			selected = append(selected, r)
		}
	}
	if len(selected) < pub.StoriesPerIssueMin {
		_, err := w.issues.ApplyTransition(ctx, issueID, issues.EventNoCandidates, issues.TransitionUpdate{})
		if err != nil {
			return fmt.Errorf("transition curating->skipped (under min): %w", err)
		}
		return nil
	}

	stories := make([]issuedoc.Story, 0, len(selected))
	summaries := make([]llmclient.Summary, 0, len(selected))
	for _, sc := range selected {
		s, err := w.summarizer.Summarize(ctx, pub.Brief, sc.Candidate)
		if err != nil {
			return fmt.Errorf("summarize candidate %s: %w", sc.Candidate.ID, err)
		}
		summaries = append(summaries, s)
		stories = append(stories, issuedoc.Story{
			ID:       sc.Candidate.ID,
			URL:      sc.Candidate.URL,
			Headline: s.Headline,
			Body:     s.Body,
		})
	}

	head, err := w.summarizer.Headline(ctx, pub.Brief, summaries)
	if err != nil {
		return fmt.Errorf("headline: %w", err)
	}

	intro := ""
	if pub.IntroEnabled {
		intro, err = w.summarizer.Intro(ctx, pub.Brief, summaries)
		if err != nil {
			return fmt.Errorf("intro: %w", err)
		}
	}

	hints := imagegen.PromptHints{IssueTitle: head.Title}
	for _, s := range summaries {
		hints.StorySummaries = append(hints.StorySummaries, s.Headline)
	}
	cover, err := w.images.Generate(ctx, pub.Brief, hints)
	if err != nil {
		return fmt.Errorf("cover image: %w", err)
	}

	doc, err := issuedoc.Assemble(issuedoc.IssueAssembly{
		Subject:  head.Subject,
		Title:    head.Title,
		CoverURL: cover.URL,
		Intro:    intro,
		Stories:  stories,
	})
	if err != nil {
		return fmt.Errorf("assemble doc: %w", err)
	}

	subject := head.Subject
	title := head.Title
	coverURL := cover.URL
	if _, err := w.issues.ApplyTransition(ctx, issueID, issues.EventCurationOK, issues.TransitionUpdate{
		Subject:  &subject,
		Title:    &title,
		CoverURL: &coverURL,
		BodyDoc:  doc,
	}); err != nil {
		return fmt.Errorf("transition curating->drafted: %w", err)
	}
	return nil
}

// ErrAlreadyCurated is returned by TriggerOnDemand when the Issue is not in
// the planned state.
var ErrAlreadyCurated = errors.New("issue is not in planned state")

// Trigger enqueues a curation.start message for issueID. The supplied
// producer is used directly so callers can share one across components.
func Trigger(producer *nsqx.Producer, issueID uuid.UUID) error {
	body, _ := json.Marshal(StartMessage{IssueID: issueID.String()})
	return producer.Publish(StartTopic, body)
}

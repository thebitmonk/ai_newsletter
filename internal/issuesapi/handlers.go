// Package issuesapi holds the HTTP handlers for Issue-related endpoints.
//
// At v1 it exposes the Issue read API (single + list), a dev-only manual
// curation trigger, and the Story / cover regenerate endpoint added in
// PRD #12 slice #14.
package issuesapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/auth"
	"github.com/thebitmonk/ai_newsletter/internal/curation"
	"github.com/thebitmonk/ai_newsletter/internal/httpx"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
)

const (
	defaultListLimit = 25
	maxListLimit     = 100
)

// Regenerator does the in-place body_doc mutations for Story / cover
// regenerate. curation.Worker satisfies this interface; tests can stub.
type Regenerator interface {
	RegenerateStorySummary(ctx context.Context, iss *issues.Issue, storyID uuid.UUID) (*issues.Issue, error)
	RegenerateCover(ctx context.Context, iss *issues.Issue) (*issues.Issue, error)
}

type Handlers struct {
	issues      *issues.Store
	triggerFn   func(uuid.UUID) error
	regenerator Regenerator
}

// NewHandlers wires the issuesapi handlers. triggerFn drives the manual
// curate endpoint (nil → 503). regenerator drives the regenerate endpoint
// (nil → 503).
func NewHandlers(is *issues.Store, triggerFn func(uuid.UUID) error, regen Regenerator) *Handlers {
	return &Handlers{issues: is, triggerFn: triggerFn, regenerator: regen}
}

// NewHandlersWithProducer is the convenience constructor for the common case
// of triggering via an nsqx.Producer and using a curation.Worker as the
// regenerator.
func NewHandlersWithProducer(is *issues.Store, producer *nsqx.Producer, regen Regenerator) *Handlers {
	var trig func(uuid.UUID) error
	if producer != nil {
		trig = func(id uuid.UUID) error { return curation.Trigger(producer, id) }
	}
	return NewHandlers(is, trig, regen)
}

func (h *Handlers) Register(r gin.IRouter) {
	r.GET("/issues/:id", h.get)
	r.GET("/publications/:id/issues", h.list)
	r.POST("/issues/:id/curate", h.curate)
	r.POST("/issues/:id/stories/:story_id/regenerate", h.regenerate)
}

// -----------------------------------------------------------------------------
// GET /issues/:id
// -----------------------------------------------------------------------------

type issueDetailResp struct {
	ID             uuid.UUID       `json:"id"`
	PublicationID  uuid.UUID       `json:"publication_id"`
	State          string          `json:"state"`
	Subject        *string         `json:"subject"`
	Title          *string         `json:"title"`
	CoverURL       *string         `json:"cover_url"`
	ScheduledAt    time.Time       `json:"scheduled_at"`
	SentAt         *time.Time      `json:"sent_at"`
	BodyDoc        json.RawMessage `json:"body_doc,omitempty"`
	BodyDocVersion int             `json:"body_doc_version"`
	FailedReason   *string         `json:"failed_reason,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func toDetail(i *issues.Issue) issueDetailResp {
	return issueDetailResp{
		ID:             i.ID,
		PublicationID:  i.PublicationID,
		State:          string(i.State),
		Subject:        i.Subject,
		Title:          i.Title,
		CoverURL:       i.CoverURL,
		ScheduledAt:    i.ScheduledAt,
		SentAt:         nil,
		BodyDoc:        i.BodyDoc,
		BodyDocVersion: i.BodyDocVersion,
		FailedReason:   i.FailedReason,
		CreatedAt:      i.CreatedAt,
		UpdatedAt:      i.UpdatedAt,
	}
}

func (h *Handlers) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "id is not a uuid")
		return
	}
	iss, err := h.issues.GetForAccount(c.Request.Context(), auth.AccountID(c), id)
	if errors.Is(err, issues.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, toDetail(iss))
}

// -----------------------------------------------------------------------------
// GET /publications/:id/issues
// -----------------------------------------------------------------------------

type issueSummaryResp struct {
	ID          uuid.UUID  `json:"id"`
	State       string     `json:"state"`
	Subject     *string    `json:"subject"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at"`
	CoverURL    *string    `json:"cover_url"`
	StoryCount  int        `json:"story_count"`
}

func toSummary(i *issues.Issue) issueSummaryResp {
	return issueSummaryResp{
		ID:          i.ID,
		State:       string(i.State),
		Subject:     i.Subject,
		ScheduledAt: i.ScheduledAt,
		SentAt:      nil,
		CoverURL:    i.CoverURL,
		StoryCount:  storyCount(i.BodyDoc),
	}
}

func storyCount(doc json.RawMessage) int {
	if len(doc) == 0 || string(doc) == "null" {
		return 0
	}
	var parsed struct {
		Content []struct {
			Type  string `json:"type"`
			Attrs struct {
				Block string `json:"block"`
			} `json:"attrs"`
		} `json:"content"`
	}
	if err := json.Unmarshal(doc, &parsed); err != nil {
		return 0
	}
	n := 0
	for _, c := range parsed.Content {
		if c.Type == "story" || c.Attrs.Block == "story" {
			n++
		}
	}
	return n
}

func (h *Handlers) list(c *gin.Context) {
	pubID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "publication id is not a uuid")
		return
	}

	filter := issues.ListFilter{}
	for _, st := range c.QueryArray("state") {
		if st == "" {
			continue
		}
		filter.States = append(filter.States, issues.State(st))
	}
	if v := c.Query("scheduled_after"); v != "" {
		t, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_scheduled_after", "must be RFC3339")
			return
		}
		filter.ScheduledAfter = &t
	}
	if v := c.Query("scheduled_before"); v != "" {
		t, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_scheduled_before", "must be RFC3339")
			return
		}
		filter.ScheduledBefore = &t
	}

	limit := defaultListLimit
	if v := c.Query("limit"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 || n > maxListLimit {
			httpx.Error(c, http.StatusBadRequest, "invalid_limit",
				fmt.Sprintf("limit must be 1..%d", maxListLimit))
			return
		}
		limit = n
	}

	var cursor *issues.ListCursor
	if v := c.Query("cursor"); v != "" {
		c2, perr := decodeCursor(v)
		if perr != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_cursor", perr.Error())
			return
		}
		cursor = c2
	}

	rows, next, err := h.issues.ListForAccount(c.Request.Context(),
		auth.AccountID(c), pubID, filter, cursor, limit)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	items := make([]issueSummaryResp, 0, len(rows))
	for i := range rows {
		items = append(items, toSummary(&rows[i]))
	}
	resp := gin.H{"items": items}
	if next != nil {
		resp["next_cursor"] = encodeCursor(*next)
	} else {
		resp["next_cursor"] = nil
	}
	c.JSON(http.StatusOK, resp)
}

func encodeCursor(c issues.ListCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*issues.ListCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	var c issues.ListCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	return &c, nil
}

// -----------------------------------------------------------------------------
// POST /issues/:id/curate
// -----------------------------------------------------------------------------

func (h *Handlers) curate(c *gin.Context) {
	if h.triggerFn == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "no_worker",
			"curation trigger is not available (no NSQ producer configured)")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "id is not a uuid")
		return
	}

	iss, err := h.issues.GetForAccount(c.Request.Context(), auth.AccountID(c), id)
	if errors.Is(err, issues.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if iss.State != issues.StatePlanned {
		httpx.Error(c, http.StatusConflict, "wrong_state",
			"issue is in state "+string(iss.State)+", curation requires planned")
		return
	}

	if err := h.triggerFn(id); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "trigger", err.Error())
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "curation_enqueued", "issue_id": id})
}

// -----------------------------------------------------------------------------
// POST /issues/:id/stories/:story_id/regenerate
// -----------------------------------------------------------------------------

type regenerateReq struct {
	Type string `json:"type"` // "summary" (default) or "image"
}

func (h *Handlers) regenerate(c *gin.Context) {
	if h.regenerator == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "no_worker",
			"regenerator is not available (LLM/image clients not configured)")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "issue id is not a uuid")
		return
	}
	storyID, err := uuid.Parse(c.Param("story_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_story_id", "story id is not a uuid")
		return
	}

	var req regenerateReq
	_ = c.ShouldBindJSON(&req) // body optional; default to summary
	if req.Type == "" {
		req.Type = "summary"
	}
	if req.Type != "summary" && req.Type != "image" {
		httpx.Error(c, http.StatusBadRequest, "invalid_type",
			"type must be 'summary' or 'image'")
		return
	}

	iss, err := h.issues.GetForAccount(c.Request.Context(), auth.AccountID(c), id)
	if errors.Is(err, issues.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "issue not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	var updated *issues.Issue
	switch req.Type {
	case "summary":
		updated, err = h.regenerator.RegenerateStorySummary(c.Request.Context(), iss, storyID)
	case "image":
		updated, err = h.regenerator.RegenerateCover(c.Request.Context(), iss)
	}
	switch {
	case errors.Is(err, curation.ErrWrongState):
		httpx.Error(c, http.StatusConflict, "wrong_state",
			"regenerate requires drafted or approved state; issue is "+string(iss.State))
		return
	case errors.Is(err, curation.ErrStoryNodeNotFound):
		httpx.Error(c, http.StatusNotFound, "story_not_found",
			"no story node with that id in this issue")
		return
	case errors.Is(err, curation.ErrCandidateExpired):
		httpx.Error(c, http.StatusGone, "candidate_expired",
			"the originating candidate for this story has expired from the pool")
		return
	case err != nil:
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	c.JSON(http.StatusOK, toDetail(updated))
}

// Package candidatesapi exposes the per-Publication Candidate pool via HTTP.
// The pool is owner-facing read-only — they can see what the poller has
// fetched and what's eligible for the next curation run.
package candidatesapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/auth"
	"github.com/thebitmonk/ai_newsletter/internal/candidates"
	"github.com/thebitmonk/ai_newsletter/internal/httpx"
)

const (
	defaultLimit = 25
	maxLimit     = 100
)

type Handlers struct {
	store *candidates.Store
}

func NewHandlers(s *candidates.Store) *Handlers {
	return &Handlers{store: s}
}

func (h *Handlers) Register(r gin.IRouter) {
	r.GET("/publications/:id/candidates", h.list)
}

type item struct {
	ID               uuid.UUID `json:"id"`
	PublicationID    uuid.UUID `json:"publication_id"`
	SourceID         uuid.UUID `json:"source_id"`
	SourceType       string    `json:"source_type"`
	SourceIdentifier string    `json:"source_identifier"`
	URL              string    `json:"url"`
	Title            string    `json:"title"`
	FetchedAt        time.Time `json:"fetched_at"`
	ExpiresAt        time.Time `json:"expires_at"`
}

func toItem(p *candidates.PoolItem) item {
	return item{
		ID:               p.ID,
		PublicationID:    p.PublicationID,
		SourceID:         p.SourceID,
		SourceType:       p.SourceType,
		SourceIdentifier: p.SourceIdentifier,
		URL:              p.URL,
		Title:            p.Title,
		FetchedAt:        p.FetchedAt,
		ExpiresAt:        p.ExpiresAt,
	}
}

func (h *Handlers) list(c *gin.Context) {
	pubID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "publication id is not a uuid")
		return
	}

	limit := defaultLimit
	if v := c.Query("limit"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 || n > maxLimit {
			httpx.Error(c, http.StatusBadRequest, "invalid_limit",
				fmt.Sprintf("limit must be 1..%d", maxLimit))
			return
		}
		limit = n
	}

	var cursor *candidates.ListCursor
	if v := c.Query("cursor"); v != "" {
		cur, perr := decodeCursor(v)
		if perr != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_cursor", perr.Error())
			return
		}
		cursor = cur
	}

	rows, next, err := h.store.ListForAccount(c.Request.Context(),
		auth.AccountID(c), pubID, cursor, limit)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	items := make([]item, 0, len(rows))
	for i := range rows {
		items = append(items, toItem(&rows[i]))
	}
	resp := gin.H{"items": items}
	if next != nil {
		resp["next_cursor"] = encodeCursor(*next)
	} else {
		resp["next_cursor"] = nil
	}
	c.JSON(http.StatusOK, resp)
}

func encodeCursor(c candidates.ListCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*candidates.ListCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	var c candidates.ListCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	return &c, nil
}

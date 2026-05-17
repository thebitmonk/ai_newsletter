package sources

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/auth"
	"github.com/thebitmonk/ai_newsletter/internal/httpx"
)

// PostCreateHook is called after a Source is successfully created. The
// poller uses this to publish a bootstrap source.poll NSQ message so the
// new Source starts polling immediately rather than waiting for the next
// supervisor sweep. Nil hook is fine (HTTP-only dev mode).
type PostCreateHook func(sourceID uuid.UUID)

type Handlers struct {
	store      *Store
	postCreate PostCreateHook
}

func NewHandlers(store *Store) *Handlers {
	return &Handlers{store: store}
}

// SetPostCreateHook installs a hook called after each successful create.
// Pass nil to clear.
func (h *Handlers) SetPostCreateHook(hook PostCreateHook) {
	h.postCreate = hook
}

// Register mounts source routes under /publications/:id/sources.
// Pass the same router group used for publications (i.e. authed v1). The
// publication id uses the same `:id` param as the publications group to avoid
// Gin's wildcard-name conflict; the source id uses `:source_id`.
func (h *Handlers) Register(r gin.IRouter) {
	g := r.Group("/publications/:id/sources")
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:source_id", h.get)
	g.PATCH("/:source_id", h.update)
	g.DELETE("/:source_id", h.delete)
}

type sourceResp struct {
	ID            uuid.UUID  `json:"id"`
	PublicationID uuid.UUID  `json:"publication_id"`
	Type          string     `json:"type"`
	Identifier    string     `json:"identifier"`
	PollInterval  string     `json:"poll_interval"`
	Enabled       bool       `json:"enabled"`
	LastPolledAt  *time.Time `json:"last_polled_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func toResp(s *Source) sourceResp {
	return sourceResp{
		ID:            s.ID,
		PublicationID: s.PublicationID,
		Type:          string(s.Type),
		Identifier:    s.Identifier,
		PollInterval:  s.PollInterval.String(),
		Enabled:       s.Enabled,
		LastPolledAt:  s.LastPolledAt,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func parsePublicationID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_publication_id", "publication id is not a uuid")
		return uuid.Nil, false
	}
	return id, true
}

func parseSourceID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("source_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_source_id", "source id is not a uuid")
		return uuid.Nil, false
	}
	return id, true
}

type createReq struct {
	Type         string  `json:"type" binding:"required"`
	Identifier   string  `json:"identifier" binding:"required"`
	PollInterval *string `json:"poll_interval"`
	Enabled      *bool   `json:"enabled"`
}

func (h *Handlers) create(c *gin.Context) {
	pubID, ok := parsePublicationID(c)
	if !ok {
		return
	}
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !IsValidType(req.Type) {
		httpx.Error(c, http.StatusBadRequest, "invalid_type",
			"type must be one of rss, youtube_channel, x_handle, substack, web")
		return
	}
	t := Type(req.Type)
	normID, err := ValidateIdentifier(t, req.Identifier)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_identifier", err.Error())
		return
	}
	interval, err := parseDurationPtr(req.PollInterval)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_poll_interval", err.Error())
		return
	}

	src, err := h.store.Create(c.Request.Context(), auth.AccountID(c), pubID, CreateParams{
		Type:         t,
		Identifier:   normID,
		PollInterval: interval,
		Enabled:      req.Enabled,
	})
	if errors.Is(err, ErrPublicationNF) {
		httpx.Error(c, http.StatusNotFound, "publication_not_found", "publication not found")
		return
	}
	if errors.Is(err, ErrDuplicate) {
		httpx.Error(c, http.StatusConflict, "duplicate_source",
			"this publication already has a source with this type and identifier")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if h.postCreate != nil {
		h.postCreate(src.ID)
	}
	c.JSON(http.StatusCreated, toResp(src))
}

func (h *Handlers) get(c *gin.Context) {
	pubID, ok := parsePublicationID(c)
	if !ok {
		return
	}
	id, ok := parseSourceID(c)
	if !ok {
		return
	}
	src, err := h.store.Get(c.Request.Context(), auth.AccountID(c), pubID, id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "source not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, toResp(src))
}

func (h *Handlers) list(c *gin.Context) {
	pubID, ok := parsePublicationID(c)
	if !ok {
		return
	}
	srcs, err := h.store.List(c.Request.Context(), auth.AccountID(c), pubID)
	if errors.Is(err, ErrPublicationNF) {
		httpx.Error(c, http.StatusNotFound, "publication_not_found", "publication not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	items := make([]sourceResp, 0, len(srcs))
	for i := range srcs {
		items = append(items, toResp(&srcs[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type updateReq struct {
	Identifier   *string `json:"identifier"`
	PollInterval *string `json:"poll_interval"`
	Enabled      *bool   `json:"enabled"`
}

func (h *Handlers) update(c *gin.Context) {
	pubID, ok := parsePublicationID(c)
	if !ok {
		return
	}
	id, ok := parseSourceID(c)
	if !ok {
		return
	}
	var req updateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// To validate the identifier we need to know the source's type, which we
	// only know after we read it. Fetch first so validation matches the type.
	existing, err := h.store.Get(c.Request.Context(), auth.AccountID(c), pubID, id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "source not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	if req.Identifier != nil {
		norm, err := ValidateIdentifier(existing.Type, *req.Identifier)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_identifier", err.Error())
			return
		}
		req.Identifier = &norm
	}
	interval, err := parseDurationPtr(req.PollInterval)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_poll_interval", err.Error())
		return
	}

	src, err := h.store.Update(c.Request.Context(), auth.AccountID(c), pubID, id, UpdateParams{
		Identifier:   req.Identifier,
		PollInterval: interval,
		Enabled:      req.Enabled,
	})
	if errors.Is(err, ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "source not found")
		return
	}
	if errors.Is(err, ErrDuplicate) {
		httpx.Error(c, http.StatusConflict, "duplicate_source",
			"this publication already has a source with this type and identifier")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, toResp(src))
}

func (h *Handlers) delete(c *gin.Context) {
	pubID, ok := parsePublicationID(c)
	if !ok {
		return
	}
	id, ok := parseSourceID(c)
	if !ok {
		return
	}
	if err := h.store.Delete(c.Request.Context(), auth.AccountID(c), pubID, id); errors.Is(err, ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "source not found")
		return
	} else if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func parseDurationPtr(s *string) (*time.Duration, error) {
	if s == nil {
		return nil, nil
	}
	d, err := time.ParseDuration(*s)
	if err != nil {
		return nil, errors.New("duration must be Go-format e.g. 1h or 30m")
	}
	if d <= 0 {
		return nil, errors.New("duration must be positive")
	}
	return &d, nil
}

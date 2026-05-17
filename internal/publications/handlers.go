package publications

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/teambition/rrule-go"

	"github.com/thebitmonk/ai_newsletter/internal/auth"
	"github.com/thebitmonk/ai_newsletter/internal/httpx"
)

type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers {
	return &Handlers{store: store}
}

func (h *Handlers) Register(r gin.IRouter) {
	g := r.Group("/publications")
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.PATCH("/:id", h.update)
	g.DELETE("/:id", h.delete)
}

type publicationResp struct {
	ID                  uuid.UUID `json:"id"`
	AccountID           uuid.UUID `json:"account_id"`
	Name                string    `json:"name"`
	Brief               string    `json:"brief"`
	Timezone            string    `json:"timezone"`
	CadenceRule         *string   `json:"cadence_rule"`
	StoriesPerIssueMin  int       `json:"stories_per_issue_min"`
	StoriesPerIssueMax  int       `json:"stories_per_issue_max"`
	IntroEnabled        bool      `json:"intro_enabled"`
	CurationLeadTime    string    `json:"curation_lead_time"`
	ApprovalGateEnabled bool      `json:"approval_gate_enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func toResp(p *Publication) publicationResp {
	return publicationResp{
		ID:                  p.ID,
		AccountID:           p.AccountID,
		Name:                p.Name,
		Brief:               p.Brief,
		Timezone:            p.Timezone,
		CadenceRule:         p.CadenceRule,
		StoriesPerIssueMin:  p.StoriesPerIssueMin,
		StoriesPerIssueMax:  p.StoriesPerIssueMax,
		IntroEnabled:        p.IntroEnabled,
		CurationLeadTime:    p.CurationLeadTime.String(),
		ApprovalGateEnabled: p.ApprovalGateEnabled,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
}

type createReq struct {
	Name                string  `json:"name" binding:"required,min=1,max=200"`
	Brief               string  `json:"brief"`
	Timezone            string  `json:"timezone" binding:"required"`
	CadenceRule         *string `json:"cadence_rule"`
	StoriesPerIssueMin  *int    `json:"stories_per_issue_min"`
	StoriesPerIssueMax  *int    `json:"stories_per_issue_max"`
	IntroEnabled        *bool   `json:"intro_enabled"`
	CurationLeadTime    *string `json:"curation_lead_time"`
	ApprovalGateEnabled *bool   `json:"approval_gate_enabled"`
}

func (h *Handlers) create(c *gin.Context) {
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateTimezone(req.Timezone); err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_timezone", err.Error())
		return
	}
	if req.CadenceRule != nil {
		if err := validateRRULE(*req.CadenceRule); err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_cadence_rule", err.Error())
			return
		}
	}
	leadTime, err := parseDurationPtr(req.CurationLeadTime)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_curation_lead_time", err.Error())
		return
	}

	pub, err := h.store.Create(c.Request.Context(), auth.AccountID(c), CreateParams{
		Name:                req.Name,
		Brief:               req.Brief,
		Timezone:            req.Timezone,
		CadenceRule:         req.CadenceRule,
		StoriesPerIssueMin:  req.StoriesPerIssueMin,
		StoriesPerIssueMax:  req.StoriesPerIssueMax,
		IntroEnabled:        req.IntroEnabled,
		CurationLeadTime:    leadTime,
		ApprovalGateEnabled: req.ApprovalGateEnabled,
	})
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusCreated, toResp(pub))
}

func (h *Handlers) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "id is not a uuid")
		return
	}
	pub, err := h.store.Get(c.Request.Context(), auth.AccountID(c), id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "publication not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, toResp(pub))
}

func (h *Handlers) list(c *gin.Context) {
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	cursor, err := DecodeCursor(c.Query("cursor"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}

	pubs, next, err := h.store.List(c.Request.Context(), auth.AccountID(c), cursor, limit)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	items := make([]publicationResp, 0, len(pubs))
	for i := range pubs {
		items = append(items, toResp(&pubs[i]))
	}
	resp := gin.H{"items": items}
	if next != nil {
		resp["next_cursor"] = EncodeCursor(*next)
	} else {
		resp["next_cursor"] = nil
	}
	c.JSON(http.StatusOK, resp)
}

type updateReq struct {
	Name                *string `json:"name"`
	Brief               *string `json:"brief"`
	Timezone            *string `json:"timezone"`
	CadenceRule         *string `json:"cadence_rule"`
	UnsetCadenceRule    bool    `json:"unset_cadence_rule"`
	StoriesPerIssueMin  *int    `json:"stories_per_issue_min"`
	StoriesPerIssueMax  *int    `json:"stories_per_issue_max"`
	IntroEnabled        *bool   `json:"intro_enabled"`
	CurationLeadTime    *string `json:"curation_lead_time"`
	ApprovalGateEnabled *bool   `json:"approval_gate_enabled"`
}

func (h *Handlers) update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "id is not a uuid")
		return
	}
	var req updateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Timezone != nil {
		if err := validateTimezone(*req.Timezone); err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_timezone", err.Error())
			return
		}
	}
	if req.CadenceRule != nil && !req.UnsetCadenceRule {
		if err := validateRRULE(*req.CadenceRule); err != nil {
			httpx.Error(c, http.StatusBadRequest, "invalid_cadence_rule", err.Error())
			return
		}
	}
	leadTime, err := parseDurationPtr(req.CurationLeadTime)
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_curation_lead_time", err.Error())
		return
	}

	pub, err := h.store.Update(c.Request.Context(), auth.AccountID(c), id, UpdateParams{
		Name:                req.Name,
		Brief:               req.Brief,
		Timezone:            req.Timezone,
		CadenceRule:         req.CadenceRule,
		UnsetCadenceRule:    req.UnsetCadenceRule,
		StoriesPerIssueMin:  req.StoriesPerIssueMin,
		StoriesPerIssueMax:  req.StoriesPerIssueMax,
		IntroEnabled:        req.IntroEnabled,
		CurationLeadTime:    leadTime,
		ApprovalGateEnabled: req.ApprovalGateEnabled,
	})
	if errors.Is(err, ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "publication not found")
		return
	}
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, toResp(pub))
}

func (h *Handlers) delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "invalid_id", "id is not a uuid")
		return
	}
	if err := h.store.Delete(c.Request.Context(), auth.AccountID(c), id); errors.Is(err, ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "publication not found")
		return
	} else if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// -----------------------------------------------------------------------------
// validation helpers
// -----------------------------------------------------------------------------

func validateTimezone(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(s); err != nil {
		return errors.New("not a valid IANA timezone")
	}
	return nil
}

func validateRRULE(s string) error {
	if _, err := rrule.StrToRRule(s); err != nil {
		return errors.New("not a valid RRULE")
	}
	return nil
}

func parseDurationPtr(s *string) (*time.Duration, error) {
	if s == nil {
		return nil, nil
	}
	d, err := time.ParseDuration(*s)
	if err != nil {
		return nil, errors.New("duration must be Go-format e.g. 24h or 30m")
	}
	if d <= 0 {
		return nil, errors.New("duration must be positive")
	}
	return &d, nil
}

func parseLimit(s string) (int, error) {
	if s == "" {
		return DefaultLimit, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}
	if n < 1 {
		return 0, errors.New("limit must be >= 1")
	}
	if n > MaxLimit {
		return 0, errors.New("limit exceeds max of 100")
	}
	return n, nil
}

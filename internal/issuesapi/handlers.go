// Package issuesapi holds the HTTP handlers for Issue-related endpoints.
//
// At v1 it exposes a dev-only manual curation trigger (POST /issues/:id/curate)
// that the curation worker consumes. The Issue read API (#11) will land here
// too once #9 is merged.
package issuesapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/auth"
	"github.com/thebitmonk/ai_newsletter/internal/curation"
	"github.com/thebitmonk/ai_newsletter/internal/httpx"
	"github.com/thebitmonk/ai_newsletter/internal/issues"
	"github.com/thebitmonk/ai_newsletter/internal/nsqx"
)

type Handlers struct {
	issues     *issues.Store
	triggerFn  func(uuid.UUID) error
}

// NewHandlers wires the issuesapi handlers. triggerFn is called by the
// manual-curate endpoint; pass nil to disable that endpoint (it will return
// 503 Service Unavailable).
func NewHandlers(is *issues.Store, triggerFn func(uuid.UUID) error) *Handlers {
	return &Handlers{issues: is, triggerFn: triggerFn}
}

// NewHandlersWithProducer is a convenience for the common case of triggering
// via an nsqx.Producer.
func NewHandlersWithProducer(is *issues.Store, producer *nsqx.Producer) *Handlers {
	var trig func(uuid.UUID) error
	if producer != nil {
		trig = func(id uuid.UUID) error { return curation.Trigger(producer, id) }
	}
	return NewHandlers(is, trig)
}

func (h *Handlers) Register(r gin.IRouter) {
	g := r.Group("/issues")
	g.POST("/:id/curate", h.curate)
}

// curate enqueues a curation.start message for the Issue. The Issue must
// belong to the requester's Account and be in the `planned` state.
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

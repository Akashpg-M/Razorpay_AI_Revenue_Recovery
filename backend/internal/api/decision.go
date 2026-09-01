package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/metrics"
	"revenue-recovery/backend/internal/orchestrator"
)

type DecisionScheduler interface {
	Schedule(context.Context, decisioning.Snapshot) (*orchestrator.ScheduledAction, error)
}

type Decision struct {
	service   *decisioning.Service
	scheduler DecisionScheduler
}

func NewDecision(service *decisioning.Service, scheduler DecisionScheduler) *Decision {
	return &Decision{service: service, scheduler: scheduler}
}

func (h *Decision) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases/:id/decision", h.decide)
}

func (h *Decision) decide(c *gin.Context) {
	started := time.Now()
	snapshot, err := h.service.Decide(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		metrics.Default.Inc("recovery_decisions_total", map[string]string{"result": "error"})
		handleServiceError(c, err)
		return
	}
	metrics.Default.Inc("recovery_decisions_total", map[string]string{"result": snapshot.Policy.Decision, "gate": snapshot.Gate.Decision})
	metrics.Default.Observe("recovery_decision", map[string]string{"result": snapshot.Policy.Decision}, time.Since(started))
	scheduled, err := h.scheduler.Schedule(c.Request.Context(), snapshot)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"decision": snapshot, "scheduled_action": scheduled})
}

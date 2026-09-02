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

type DecisionCoordinator interface {
	Decide(context.Context, domain.ID) (decisioning.Snapshot, *orchestrator.ScheduledAction, error)
}

type Decision struct {
	coordinator DecisionCoordinator
}

func NewDecision(coordinator DecisionCoordinator) *Decision {
	return &Decision{coordinator: coordinator}
}

func (h *Decision) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases/:id/decision", h.decide)
}

func (h *Decision) decide(c *gin.Context) {
	started := time.Now()
	snapshot, scheduled, err := h.coordinator.Decide(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		metrics.Default.Inc("recovery_decisions_total", map[string]string{"result": "error"})
		handleServiceError(c, err)
		return
	}
	metrics.Default.Inc("recovery_decisions_total", map[string]string{"result": snapshot.Policy.Decision, "gate": snapshot.Gate.Decision})
	metrics.Default.Observe("recovery_decision", map[string]string{"result": snapshot.Policy.Decision}, time.Since(started))
	c.JSON(http.StatusCreated, gin.H{"decision": snapshot, "scheduled_action": scheduled})
}

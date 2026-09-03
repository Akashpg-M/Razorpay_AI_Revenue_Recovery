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
type ObjectiveComparator interface {
	CompareObjectives(context.Context, domain.ID) ([]decisioning.ObjectiveComparison, error)
}

type Decision struct {
	coordinator DecisionCoordinator
	comparator  ObjectiveComparator
}

func NewDecision(coordinator DecisionCoordinator, comparator ObjectiveComparator) *Decision {
	return &Decision{coordinator: coordinator, comparator: comparator}
}

func (h *Decision) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases/:id/decision", h.decide)
	router.GET("/recovery-cases/:id/objective-comparison", h.compareObjectives)
}

func (h *Decision) compareObjectives(c *gin.Context) {
	rows, err := h.comparator.CompareObjectives(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": "EXPLANATION_ONLY", "persisted": false, "objectives": rows})
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

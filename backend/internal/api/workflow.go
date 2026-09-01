package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/orchestrator"
)

type WorkflowStore interface {
	GetWorkflow(context.Context, domain.ID) (orchestrator.WorkflowView, error)
}
type Workflow struct{ store WorkflowStore }

func NewWorkflow(store WorkflowStore) *Workflow { return &Workflow{store: store} }
func (h *Workflow) Register(router *gin.RouterGroup) {
	router.GET("/recovery-cases/:id/workflow", h.get)
}
func (h *Workflow) get(c *gin.Context) {
	result, err := h.store.GetWorkflow(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

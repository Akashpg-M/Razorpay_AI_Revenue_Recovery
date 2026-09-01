package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/resilience"
)

type ResilienceStore interface {
	SaveResilienceRun(context.Context, resilience.Run) error
	GetResilienceRun(context.Context, string) (resilience.Run, error)
}
type Resilience struct {
	store       ResilienceStore
	environment string
	enabled     bool
}

func NewResilience(store ResilienceStore, environment string) *Resilience {
	return &Resilience{store: store, environment: environment, enabled: environment == "development" || environment == "demo" || environment == "test"}
}
func (h *Resilience) Register(router *gin.RouterGroup) {
	router.POST("/resilience/scenarios/:scenario/run", h.run)
	router.GET("/resilience/runs/:id", h.get)
}
func (h *Resilience) guard(c *gin.Context) bool {
	if !h.enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "resilience lab is disabled outside development, demo, and test environments"})
		return false
	}
	return true
}
func (h *Resilience) run(c *gin.Context) {
	if !h.guard(c) {
		return
	}
	scenario := strings.ToLower(strings.TrimSpace(c.Param("scenario")))
	started := time.Now().UTC()
	result := resilience.RunFaultScenario(scenario)
	if result.Error == "unknown fault mode" {
		c.JSON(http.StatusBadRequest, gin.H{"error": result.Error})
		return
	}
	run := resilience.Run{ID: id.New(), Environment: h.environment, Result: result, StartedAt: started, CompletedAt: time.Now().UTC()}
	if err := h.store.SaveResilienceRun(c.Request.Context(), run); err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, run)
}
func (h *Resilience) get(c *gin.Context) {
	if !h.guard(c) {
		return
	}
	run, err := h.store.GetResilienceRun(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, run)
}

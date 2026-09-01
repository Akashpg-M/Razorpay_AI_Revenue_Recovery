package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/budget"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/portfolio"
)

type Portfolio struct {
	priorities *portfolio.Service
	budgets    *budget.Service
}

func NewPortfolio(priorities *portfolio.Service, budgets *budget.Service) *Portfolio {
	return &Portfolio{priorities: priorities, budgets: budgets}
}
func (h *Portfolio) Register(router *gin.RouterGroup) {
	router.POST("/merchants/:id/portfolio-priority-runs", h.priority)
	router.POST("/merchants/:id/budget-allocation-runs", h.allocate)
}
func (h *Portfolio) priority(c *gin.Context) {
	runID, items, err := h.priorities.Run(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"run_id": runID, "algorithm_version": portfolio.AlgorithmVersion, "priorities": items})
}
func (h *Portfolio) allocate(c *gin.Context) {
	var input struct {
		PriorityRunID string        `json:"priority_run_id" binding:"required"`
		Algorithm     string        `json:"algorithm"`
		Budget        budget.Limits `json:"budget"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Budget.SpendMinor < 0 || input.Budget.Contacts < 0 || input.Budget.Retries < 0 || input.Budget.DiscountMinor < 0 || input.Budget.HumanReviews < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "budget limits must be non-negative"})
		return
	}
	result, err := h.budgets.Run(c.Request.Context(), domain.ID(c.Param("id")), input.PriorityRunID, input.Algorithm, input.Budget)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"allocation_run": result})
}

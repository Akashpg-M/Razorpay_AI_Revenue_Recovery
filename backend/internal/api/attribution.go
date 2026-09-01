package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/attribution"
	"revenue-recovery/backend/internal/domain"
)

type Attributions struct{ service *attribution.Service }

func NewAttributions(service *attribution.Service) *Attributions {
	return &Attributions{service: service}
}
func (h *Attributions) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases/:id/recovery-observations", h.observe)
	router.GET("/recovery-cases/:id/attributions", h.list)
}
func (h *Attributions) observe(c *gin.Context) {
	var input attribution.ObserveInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.CaseID = domain.ID(c.Param("id"))
	result, created, err := h.service.Observe(c.Request.Context(), input)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"attribution": result, "created": created})
}
func (h *Attributions) list(c *gin.Context) {
	result, err := h.service.List(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"attributions": result})
}

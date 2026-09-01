package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/eligibility"
)

type Eligibility struct{ service *eligibility.Service }

func NewEligibility(service *eligibility.Service) *Eligibility { return &Eligibility{service: service} }
func (h *Eligibility) Register(router *gin.RouterGroup) {
	router.GET("/recovery-cases/:id/eligibility", h.get)
}
func (h *Eligibility) get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/intelligence"
)

type Intelligence struct{ service *intelligence.Service }

func NewIntelligence(service *intelligence.Service) *Intelligence {
	return &Intelligence{service: service}
}
func (h *Intelligence) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases/:id/predictions", h.predict)
}
func (h *Intelligence) predict(c *gin.Context) {
	result, err := h.service.Predict(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		problem(c, http.StatusUnprocessableEntity, "prediction_failed", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

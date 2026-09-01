package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/responses"
)

type CustomerResponses struct{ service *responses.Service }

func NewCustomerResponses(service *responses.Service) *CustomerResponses {
	return &CustomerResponses{service: service}
}
func (h *CustomerResponses) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases/:id/responses", h.ingest)
}

func (h *CustomerResponses) ingest(c *gin.Context) {
	var input struct {
		Type          responses.Type `json:"response_type" binding:"required"`
		Payload       map[string]any `json:"payload"`
		Source        string         `json:"source" binding:"required"`
		CorrelationID string         `json:"correlation_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	payload, _ := json.Marshal(input.Payload)
	result, created, err := h.service.Ingest(c.Request.Context(), responses.Response{CaseID: domain.ID(c.Param("id")), Type: input.Type, Payload: payload, Source: input.Source, CorrelationID: input.CorrelationID})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"response": result, "created": created})
}

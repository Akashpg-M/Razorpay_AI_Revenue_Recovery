package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/promises"
)

type Promises struct{ service *promises.Service }

func NewPromises(service *promises.Service) *Promises { return &Promises{service: service} }

func (h *Promises) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases/:id/promises", h.create)
	router.GET("/recovery-cases/:id/promises", h.list)
	router.GET("/promises/:id", h.get)
	router.POST("/promises/:id/cancel", h.cancel)
}

func (h *Promises) create(c *gin.Context) {
	var input struct {
		Text                string          `json:"text"`
		PromisedFor         *time.Time      `json:"promised_for"`
		PromisedAmountMinor *int64          `json:"promised_amount_minor"`
		Source              json.RawMessage `json:"source"`
		CorrelationID       string          `json:"correlation_id"`
		Timezone            string          `json:"timezone"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, created, err := h.service.Create(c.Request.Context(), promises.CreateInput{CaseID: domain.ID(c.Param("id")), Text: input.Text, PromisedFor: input.PromisedFor, PromisedAmountMinor: input.PromisedAmountMinor, Source: input.Source, CorrelationID: input.CorrelationID, Timezone: input.Timezone})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"promise": result, "created": created})
}

func (h *Promises) list(c *gin.Context) {
	result, err := h.service.List(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"promises": result})
}

func (h *Promises) get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"promise": result})
}

func (h *Promises) cancel(c *gin.Context) {
	var input struct {
		CorrelationID string `json:"correlation_id"`
	}
	_ = c.ShouldBindJSON(&input)
	result, err := h.service.Cancel(c.Request.Context(), domain.ID(c.Param("id")), input.CorrelationID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"promise": result})
}

package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/recovery"
)

type RecoveryCases struct{ service *recovery.Service }

func NewRecoveryCases(service *recovery.Service) *RecoveryCases {
	return &RecoveryCases{service: service}
}

func (h *RecoveryCases) Register(router *gin.RouterGroup) {
	router.POST("/recovery-cases", h.create)
	router.GET("/recovery-cases/:id", h.get)
	router.POST("/recovery-cases/:id/transitions", h.transition)
	router.GET("/recovery-cases/:id/events", h.events)
	router.POST("/recovery-cases/:id/events", h.recordEvent)
}

func (h *RecoveryCases) create(c *gin.Context) {
	var input recovery.CreateCaseInput
	if err := c.ShouldBindJSON(&input); err != nil {
		problem(c, http.StatusBadRequest, "invalid_request", err)
		return
	}
	result, err := h.service.CreateCase(c.Request.Context(), input)
	if err != nil {
		problem(c, http.StatusUnprocessableEntity, "case_creation_failed", err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *RecoveryCases) get(c *gin.Context) {
	result, err := h.service.GetCase(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *RecoveryCases) transition(c *gin.Context) {
	var input recovery.TransitionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		problem(c, http.StatusBadRequest, "invalid_request", err)
		return
	}
	result, err := h.service.Transition(c.Request.Context(), domain.ID(c.Param("id")), input)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *RecoveryCases) events(c *gin.Context) {
	result, err := h.service.Events(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"case_id": c.Param("id"), "events": result})
}

func (h *RecoveryCases) recordEvent(c *gin.Context) {
	var input recovery.RecordEventInput
	if err := c.ShouldBindJSON(&input); err != nil {
		problem(c, http.StatusBadRequest, "invalid_request", err)
		return
	}
	result, err := h.service.RecordEvent(c.Request.Context(), domain.ID(c.Param("id")), input)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, recovery.ErrNotFound):
		problem(c, http.StatusNotFound, "not_found", err)
	case errors.Is(err, recovery.ErrConflict):
		problem(c, http.StatusConflict, "version_conflict", err)
	case strings.Contains(err.Error(), "invalid recovery case transition"):
		problem(c, http.StatusUnprocessableEntity, "invalid_state_transition", err)
	default:
		problem(c, http.StatusUnprocessableEntity, "operation_failed", err)
	}
}

func problem(c *gin.Context, status int, code string, err error) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": err.Error()}})
}

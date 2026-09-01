package api

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/modelregistry"
)

type ModelRegistry struct{ service *modelregistry.Service }

func NewModelRegistry(service *modelregistry.Service) *ModelRegistry {
	return &ModelRegistry{service: service}
}
func (h *ModelRegistry) Register(router *gin.RouterGroup) {
	router.POST("/model-registry/candidates", h.create)
	router.GET("/model-registry/:id", h.get)
	router.POST("/model-registry/:id/status", h.status)
}
func (h *ModelRegistry) create(c *gin.Context) {
	var input struct {
		Entry modelregistry.Entry `json:"entry"`
		Actor json.RawMessage     `json:"actor"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Create(c.Request.Context(), input.Entry, input.Actor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"model": result})
}
func (h *ModelRegistry) get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"model": result})
}
func (h *ModelRegistry) status(c *gin.Context) {
	var input modelregistry.StatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Transition(c.Request.Context(), domain.ID(c.Param("id")), input)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"model": result})
}

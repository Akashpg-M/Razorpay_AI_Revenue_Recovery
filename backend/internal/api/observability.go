package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/observability"
)

type Observability struct{ service *observability.Service }

func NewObservability(service *observability.Service) *Observability { return &Observability{service} }
func (h *Observability) Register(r *gin.RouterGroup)                 { r.GET("/observability", h.get) }
func (h *Observability) get(c *gin.Context) {
	value, err := h.service.Snapshot(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "observability snapshot unavailable"})
		return
	}
	c.JSON(http.StatusOK, value)
}

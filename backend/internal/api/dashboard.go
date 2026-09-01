package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/reporting"
)

type Dashboard struct{ service *reporting.Service }

func NewDashboard(service *reporting.Service) *Dashboard { return &Dashboard{service: service} }
func (h *Dashboard) Register(router *gin.RouterGroup)    { router.GET("/dashboard", h.get) }
func (h *Dashboard) get(c *gin.Context) {
	value, err := h.service.Get(c.Request.Context())
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, value)
}

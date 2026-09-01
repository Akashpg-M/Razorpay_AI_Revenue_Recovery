package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/domain"
)

type Context struct{ service *recoverycontext.Service }

func NewContext(service *recoverycontext.Service) *Context { return &Context{service: service} }
func (h *Context) Register(router *gin.RouterGroup)        { router.GET("/recovery-cases/:id/context", h.get) }
func (h *Context) get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

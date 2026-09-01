package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/replay"
)

type Replay struct{ store replay.Store }

func NewReplay(store replay.Store) *Replay         { return &Replay{store: store} }
func (h *Replay) Register(router *gin.RouterGroup) { router.GET("/recovery-cases/:id/replay", h.get) }
func (h *Replay) get(c *gin.Context) {
	value, err := h.store.GetReplay(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, value)
}

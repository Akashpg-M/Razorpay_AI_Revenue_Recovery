package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/merchantprofile"
)

type MerchantProfiles struct{ service *merchantprofile.Service }

func NewMerchantProfiles(service *merchantprofile.Service) *MerchantProfiles {
	return &MerchantProfiles{service: service}
}
func (h *MerchantProfiles) Register(router *gin.RouterGroup) {
	router.GET("/merchants/:id/optimization-profile", h.get)
	router.POST("/merchants/:id/optimization-profiles", h.create)
}
func (h *MerchantProfiles) get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": result})
}
func (h *MerchantProfiles) create(c *gin.Context) {
	var input domain.MerchantOptimizationProfile
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.MerchantID = domain.ID(c.Param("id"))
	result, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"profile": result})
}

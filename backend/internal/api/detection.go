package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"revenue-recovery/backend/internal/detection"
	"revenue-recovery/backend/internal/integrations/razorpay"
	"revenue-recovery/backend/internal/metrics"
)

type Detection struct {
	service  *detection.Service
	checkout detection.CheckoutAdapter
	razorpay *razorpay.Ingestor
}

func NewDetection(service *detection.Service, checkout detection.CheckoutAdapter, razorpayIngestor *razorpay.Ingestor) *Detection {
	return &Detection{service: service, checkout: checkout, razorpay: razorpayIngestor}
}
func (h *Detection) Register(router *gin.RouterGroup) {
	router.POST("/detection/subscription", h.subscription)
	router.POST("/detection/checkout", h.checkoutEvent)
	router.POST("/webhooks/razorpay", h.razorpayWebhook)
}
func (h *Detection) subscription(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		problem(c, 400, "invalid_request", err)
		return
	}
	result, err := h.service.Detect(c.Request.Context(), detection.SubscriptionAdapter{Provider: "normalized"}, body, c.GetHeader("X-Event-Id"))
	if err != nil {
		problem(c, 422, "detection_failed", err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (h *Detection) checkoutEvent(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		problem(c, 400, "invalid_request", err)
		return
	}
	result, err := h.service.Detect(c.Request.Context(), h.checkout, json.RawMessage(body), c.GetHeader("X-Event-Id"))
	if err != nil {
		problem(c, 422, "detection_failed", err)
		return
	}
	status := http.StatusOK
	if !result.RiskDetected {
		status = http.StatusAccepted
	}
	c.JSON(status, result)
}
func (h *Detection) razorpayWebhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20))
	if err != nil {
		problem(c, 400, "invalid_request", err)
		return
	}
	result, duplicate, err := h.razorpay.Ingest(c.Request.Context(), body, c.GetHeader("X-Razorpay-Signature"), c.GetHeader("X-Razorpay-Event-Id"))
	if err != nil {
		metrics.Default.Inc("recovery_webhooks_total", map[string]string{"provider": "razorpay", "result": "rejected"})
		status := http.StatusUnprocessableEntity
		if err == razorpay.ErrInvalidSignature {
			status = http.StatusUnauthorized
		}
		problem(c, status, "webhook_rejected", err)
		return
	}
	resultLabel := "accepted"
	if duplicate {
		resultLabel = "duplicate"
	}
	metrics.Default.Inc("recovery_webhooks_total", map[string]string{"provider": "razorpay", "result": resultLabel})
	c.JSON(http.StatusOK, gin.H{"duplicate": duplicate, "result": result})
}

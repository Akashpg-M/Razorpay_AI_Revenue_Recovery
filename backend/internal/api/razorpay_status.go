package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"revenue-recovery/backend/internal/integrations/razorpay"
)

type RazorpayStatus struct {
	client                    *razorpay.Client
	provider                  string
	webhookConfigured         bool
	externalWebhookConfigured bool
}

func NewRazorpayStatus(client *razorpay.Client, provider string, webhookConfigured, externalWebhookConfigured bool) *RazorpayStatus {
	return &RazorpayStatus{client: client, provider: provider, webhookConfigured: webhookConfigured, externalWebhookConfigured: externalWebhookConfigured}
}
func (h *RazorpayStatus) Register(router *gin.RouterGroup) {
	router.GET("/integrations/razorpay/status", h.get)
}
func (h *RazorpayStatus) get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	status := h.client.CheckAuthentication(ctx)
	c.JSON(http.StatusOK, gin.H{
		"configured": status.Configured, "mode": status.Mode, "selected_provider": h.provider,
		"reachable": status.Reachable, "authenticated": status.Authenticated,
		"http_status": status.HTTPStatus, "error_code": status.ErrorCode,
		"api_base_url": h.client.BaseURL(), "payment_link_supported": true,
		"payment_link_fetch_supported": true, "payment_lookup_supported": true,
		"subscription_lookup_supported":   false,
		"webhook_verification_configured": h.webhookConfigured,
		// This records explicit public-URL configuration, not proof that the
		// Razorpay Dashboard can reach or is configured to use that URL.
		"external_webhook_delivery_configured": h.externalWebhookConfigured,
	})
}

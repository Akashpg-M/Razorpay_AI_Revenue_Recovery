package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/detection"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/promises"
)

type demoDecisionCoordinator interface {
	Decide(context.Context, domain.ID) (decisioning.Snapshot, *orchestrator.ScheduledAction, error)
}

type DemoScenarios struct {
	enabled   bool
	detection *detection.Service
	checkout  detection.CheckoutAdapter
	decisions demoDecisionCoordinator
	promises  *promises.Service
	now       func() time.Time
}

func NewDemoScenarios(environment string, detector *detection.Service, checkout detection.CheckoutAdapter, decisions demoDecisionCoordinator, promiseService *promises.Service) *DemoScenarios {
	allowed := map[string]bool{"development": true, "demo": true, "test": true}
	return &DemoScenarios{enabled: allowed[strings.ToLower(environment)], detection: detector, checkout: checkout, decisions: decisions, promises: promiseService, now: time.Now}
}

func (h *DemoScenarios) Register(router *gin.RouterGroup) {
	router.POST("/demo/scenarios/:scenario", h.create)
}

func (h *DemoScenarios) create(c *gin.Context) {
	if !h.enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "demo scenario creation is disabled outside development, demo, and test environments"})
		return
	}
	if h.detection == nil || h.decisions == nil {
		handleServiceError(c, errors.New("demo scenario services are unavailable"))
		return
	}

	scenario := strings.ToLower(strings.TrimSpace(c.Param("scenario")))
	now := h.now().UTC()
	correlationID := "demo:" + id.New()
	var result detection.Result
	var err error

	switch scenario {
	case "high_value_checkout", "checkout_abandonment":
		amount := int64(899900)
		if scenario == "checkout_abandonment" {
			amount = 249900
		}
		checkoutID := "checkout_" + id.New()
		started, _ := json.Marshal(detection.CheckoutEvent{EventType: "CHECKOUT_STARTED", CheckoutID: checkoutID,
			MerchantID: "demo-merchant-v1", CustomerID: "demo-customer-v1", AmountMinor: amount, Currency: "INR",
			CheckoutStage: "PAYMENT", OccurredAt: now, ValidUntil: now.Add(7 * 24 * time.Hour)})
		if _, err = h.detection.Detect(c.Request.Context(), h.checkout, started, correlationID+":started"); err != nil {
			handleServiceError(c, err)
			return
		}
		abandoned, _ := json.Marshal(detection.CheckoutEvent{EventType: "CHECKOUT_ABANDONED", CheckoutID: checkoutID,
			AbandonmentReason: "payment_friction", OccurredAt: now.Add(time.Second)})
		result, err = h.detection.Detect(c.Request.Context(), h.checkout, abandoned, correlationID+":abandoned")
	case "payment_failure", "active_promise", "low_value":
		amount := int64(129900)
		failureCode := "insufficient_funds"
		if scenario == "low_value" {
			amount = 500
		}
		paymentID := "pay_demo_" + id.New()
		payload, _ := json.Marshal(detection.SubscriptionEvent{EventType: "payment.failed", MerchantID: "demo-merchant-v1",
			CustomerID: "demo-customer-v1", AmountMinor: amount, Currency: "INR", PaymentID: paymentID,
			FailureCode: failureCode, OccurredAt: now, RecoveryWindowHours: 168,
			Metadata: json.RawMessage(`{"source":"frontend-scenario-lab"}`)})
		result, err = h.detection.Detect(c.Request.Context(), detection.SubscriptionAdapter{Provider: "demo-scenario"}, payload, correlationID)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported business scenario", "supported": []string{"high_value_checkout", "checkout_abandonment", "payment_failure", "active_promise", "low_value"}})
		return
	}
	if err != nil {
		handleServiceError(c, err)
		return
	}

	var promise any
	if scenario == "active_promise" {
		promisedFor := now.Add(24 * time.Hour)
		created, _, createErr := h.promises.Create(c.Request.Context(), promises.CreateInput{CaseID: result.Case.ID,
			PromisedFor: &promisedFor, PromisedAmountMinor: &result.Case.AmountAtRiskMinor, Timezone: "UTC",
			Source: json.RawMessage(`{"source":"frontend-scenario-lab"}`), CorrelationID: correlationID + ":promise"})
		if createErr != nil {
			handleServiceError(c, createErr)
			return
		}
		promise = created
	}

	snapshot, scheduled, err := h.decisions.Decide(c.Request.Context(), result.Case.ID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	current := result.Case
	// The replay endpoint is authoritative after decisioning; this response
	// exposes the expected state without duplicating another read dependency.
	if snapshot.Policy.Decision == "ESCALATE" {
		current.CurrentState = domain.StateEscalated
	} else if scheduled != nil {
		current.CurrentState = domain.StateScheduled
	} else {
		current.CurrentState = domain.StateActionPending
	}
	c.JSON(http.StatusCreated, gin.H{"scenario": scenario, "data_mode": "LIVE_OPERATIONAL_SYNTHETIC_INPUT", "case": current,
		"decision": snapshot, "scheduled_action": scheduled, "promise": promise, "correlation_id": correlationID})
}

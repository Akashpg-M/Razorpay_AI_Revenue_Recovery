package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/metrics"
	"revenue-recovery/backend/internal/operations"
)

type Operations struct{ service *operations.Service }

func NewOperations(service *operations.Service) *Operations { return &Operations{service: service} }
func (h *Operations) Register(router *gin.RouterGroup) {
	router.GET("/operations/recovery-queue", h.list)
	router.GET("/operations/recovery-queue/metrics", h.metrics)
	router.GET("/operations/recovery-queue/:id", h.get)
	router.POST("/operations/recovery-queue/:id/review", h.review)
}
func (h *Operations) list(c *gin.Context) {
	filter := operations.Filter{Category: c.Query("category"), MerchantID: c.Query("merchant_id"), Status: c.Query("status"), Priority: c.Query("priority"), LeakType: c.Query("leak_type"), Sort: c.Query("sort")}
	filter.MinAmountMinor, _ = strconv.ParseInt(c.Query("min_amount_minor"), 10, 64)
	filter.MaxAmountMinor, _ = strconv.ParseInt(c.Query("max_amount_minor"), 10, 64)
	if raw := c.Query("deadline_before"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			problem(c, http.StatusBadRequest, "invalid_deadline", err)
			return
		}
		filter.DeadlineBefore = &value
	}
	items, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items), "sort": "deadline_asc,nerv_desc,case_id"})
}
func (h *Operations) get(c *gin.Context) {
	item, reviews, err := h.service.Get(c.Request.Context(), domain.ID(c.Param("id")))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": item, "reviews": reviews})
}
func (h *Operations) metrics(c *gin.Context) {
	value, err := h.service.Metrics(c.Request.Context())
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, value)
}
func (h *Operations) review(c *gin.Context) {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var input operations.ReviewInput
	if err := decoder.Decode(&input); err != nil {
		problem(c, http.StatusBadRequest, "invalid_review", err)
		return
	}
	input.OperatorID = strings.TrimSpace(input.OperatorID)
	review, scheduled, created, err := h.service.Review(c.Request.Context(), domain.ID(c.Param("id")), input)
	if err != nil {
		metrics.Default.Inc("recovery_human_reviews_total", map[string]string{"decision": string(input.Decision), "result": "error"})
		handleServiceError(c, err)
		return
	}
	metrics.Default.Inc("recovery_human_reviews_total", map[string]string{"decision": string(review.Decision), "result": review.ReauthorizationResult})
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	next := "REMAINS_IN_REVIEW"
	if review.Decision == operations.Approve && scheduled != nil {
		next = "ACTION_SCHEDULED_AFTER_REAUTHORIZATION"
	} else if review.ReauthorizationResult == "STALE_APPROVAL" || review.ReauthorizationResult == "DENIED" || review.ReauthorizationResult == "STOPPED" {
		next = "NO_EXECUTION_REASSESS_REQUIRED"
	} else if review.Decision == operations.Stop {
		next = "CASE_STOPPED"
	} else if review.Decision == operations.Defer {
		next = "DEFERRED_UNTIL_REVIEW_AFTER"
	}
	c.JSON(status, gin.H{"review": review, "scheduled_action": scheduled, "created": created, "reauthorization_passed": scheduled != nil, "next_step": next})
}

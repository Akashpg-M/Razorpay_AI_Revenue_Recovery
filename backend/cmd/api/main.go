package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	apihttp "revenue-recovery/backend/internal/api"
	"revenue-recovery/backend/internal/attribution"
	"revenue-recovery/backend/internal/budget"
	"revenue-recovery/backend/internal/config"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/correlation"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/detection"
	"revenue-recovery/backend/internal/eligibility"
	"revenue-recovery/backend/internal/integrations/razorpay"
	"revenue-recovery/backend/internal/intelligence"
	"revenue-recovery/backend/internal/logging"
	"revenue-recovery/backend/internal/merchantprofile"
	"revenue-recovery/backend/internal/metrics"
	"revenue-recovery/backend/internal/modelregistry"
	"revenue-recovery/backend/internal/observability"
	"revenue-recovery/backend/internal/operations"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/portfolio"
	"revenue-recovery/backend/internal/promises"
	"revenue-recovery/backend/internal/recovery"
	"revenue-recovery/backend/internal/reporting"
	"revenue-recovery/backend/internal/responses"
	"revenue-recovery/backend/internal/store"
)

func main() {
	cfg := config.Load()
	logger := logging.New()

	ctx := context.Background()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error(
			"failed_to_initialize_database",
			"error",
			err,
		)
		panic(err)
	}
	defer db.Close()

	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error(
			"failed_to_parse_redis_url",
			"error",
			err,
		)
		panic(err)
	}

	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()

	decisionClient := decisionclient.New(
		cfg.DecisionServiceURL,
	)

	router := gin.New()

	router.Use(gin.Recovery())
	router.Use(cors(cfg.FrontendOrigin))
	router.Use(correlation.Middleware())
	router.Use(requestLogger(logger))
	router.GET("/metrics", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(metrics.Default.Prometheus()))
	})

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "backend",
			"status":  "ok",
		})
	})

	router.GET("/health/ready", func(c *gin.Context) {
		checkCtx, cancel := context.WithTimeout(
			c.Request.Context(),
			2*time.Second,
		)
		defer cancel()

		checks := gin.H{
			"postgres":         "ok",
			"redis":            "ok",
			"decision_service": "ok",
		}

		ready := true

		if err := db.Ping(checkCtx); err != nil {
			checks["postgres"] = "error"
			ready = false
		}

		if err := redisClient.Ping(checkCtx).Err(); err != nil {
			checks["redis"] = "optional_unavailable"
		}

		if err := decisionClient.Health(checkCtx); err != nil {
			checks["decision_service"] = "error"
			ready = false
		}
		var schema string
		if err := db.QueryRow(checkCtx, `SELECT value FROM platform_metadata WHERE key='schema_version'`).Scan(&schema); err != nil || schema != "phase_55" {
			checks["schema"] = "migration_required"
			ready = false
		} else {
			checks["schema"] = schema
		}

		status := http.StatusOK

		if !ready {
			status = http.StatusServiceUnavailable
		}

		c.JSON(status, gin.H{
			"service": "backend",
			"status": func() string {
				if ready {
					return "ready"
				}
				return "not_ready"
			}(),
			"checks": checks,
		})
	})

	router.GET(
		"/api/v1/system/decision-service",
		func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(
				c.Request.Context(),
				2*time.Second,
			)
			defer cancel()

			if err := decisionClient.Health(ctx); err != nil {
				c.JSON(
					http.StatusServiceUnavailable,
					gin.H{
						"status": "unavailable",
						"error":  err.Error(),
					},
				)
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"status": "connected",
			})
		},
	)

	recoveryRepository := store.NewPostgres(db)
	razorpayClient := razorpay.NewClient(cfg.RazorpayAPIURL, cfg.RazorpayKeyID, cfg.RazorpayKeySecret)
	razorpayStatusHandlers := apihttp.NewRazorpayStatus(razorpayClient, cfg.PaymentProvider, cfg.RazorpayWebhookSecret != "", cfg.RazorpayWebhookPublicURL != "")
	razorpayStatusHandlers.Register(router.Group("/api/v1"))
	recoveryService := recovery.NewService(recoveryRepository)
	recoveryHandlers := apihttp.NewRecoveryCases(recoveryService)
	recoveryHandlers.Register(router.Group("/api/v1"))
	detectionService := detection.NewService(recoveryService)
	razorpayIngestor := razorpay.NewIngestor(cfg.RazorpayWebhookSecret, recoveryRepository, detectionService)
	attributionService := attribution.NewService(recoveryRepository)
	razorpayIngestor.SetRecoveryObserver(attributionService, recoveryRepository)
	detectionHandlers := apihttp.NewDetection(detectionService, detection.CheckoutAdapter{Store: recoveryRepository}, razorpayIngestor)
	detectionHandlers.Register(router.Group("/api/v1"))
	contextService := recoverycontext.NewService(recoveryRepository)
	contextHandlers := apihttp.NewContext(contextService)
	contextHandlers.Register(router.Group("/api/v1"))
	eligibilityService := eligibility.NewService(contextService)
	eligibilityHandlers := apihttp.NewEligibility(eligibilityService)
	eligibilityHandlers.Register(router.Group("/api/v1"))
	intelligenceService := intelligence.NewService(contextService, decisionClient, recoveryRepository)
	intelligenceHandlers := apihttp.NewIntelligence(intelligenceService)
	intelligenceHandlers.Register(router.Group("/api/v1"))
	decisionService := decisioning.NewService(contextService, decisionClient, recoveryRepository)
	decisionCoordinator := orchestrator.NewDecisionCoordinator(decisionService, recoveryRepository)
	decisionHandlers := apihttp.NewDecision(decisionCoordinator, decisionService)
	decisionHandlers.Register(router.Group("/api/v1"))
	responseService := responses.NewService(recoveryRepository)
	promiseService := promises.NewService(recoveryRepository, nil)
	promiseService.SetReassessor(decisionCoordinator)
	responseService.SetPromiseCreator(promiseService)
	responseHandlers := apihttp.NewCustomerResponses(responseService)
	responseHandlers.Register(router.Group("/api/v1"))
	promiseHandlers := apihttp.NewPromises(promiseService)
	promiseHandlers.Register(router.Group("/api/v1"))
	demoScenarioHandlers := apihttp.NewDemoScenarios(cfg.Environment, detectionService, detection.CheckoutAdapter{Store: recoveryRepository}, decisionCoordinator, promiseService)
	demoScenarioHandlers.Register(router.Group("/api/v1"))
	merchantProfileService := merchantprofile.NewService(recoveryRepository)
	merchantProfileHandlers := apihttp.NewMerchantProfiles(merchantProfileService)
	merchantProfileHandlers.Register(router.Group("/api/v1"))
	attributionHandlers := apihttp.NewAttributions(attributionService)
	attributionHandlers.Register(router.Group("/api/v1"))
	portfolioHandlers := apihttp.NewPortfolio(portfolio.NewService(recoveryRepository), budget.NewService(recoveryRepository))
	portfolioHandlers.Register(router.Group("/api/v1"))
	modelRegistryHandlers := apihttp.NewModelRegistry(modelregistry.NewService(recoveryRepository))
	modelRegistryHandlers.Register(router.Group("/api/v1"))
	workflowHandlers := apihttp.NewWorkflow(recoveryRepository)
	workflowHandlers.Register(router.Group("/api/v1"))
	resilienceHandlers := apihttp.NewResilience(recoveryRepository, cfg.Environment)
	resilienceHandlers.Register(router.Group("/api/v1"))
	dashboardHandlers := apihttp.NewDashboard(reporting.NewService(recoveryRepository, cfg.EvaluationResultsPath))
	dashboardHandlers.Register(router.Group("/api/v1"))
	replayHandlers := apihttp.NewReplay(recoveryRepository)
	replayHandlers.Register(router.Group("/api/v1"))
	operationsHandlers := apihttp.NewOperations(operations.NewService(recoveryRepository, contextService))
	operationsHandlers.Register(router.Group("/api/v1"))
	observabilityHandlers := apihttp.NewObservability(observability.New(recoveryRepository))
	observabilityHandlers.Register(router.Group("/api/v1"))

	logger.Info(
		"backend_starting",
		"port",
		cfg.Port,
		"environment",
		cfg.Environment,
	)

	if err := router.Run(":" + cfg.Port); err != nil {
		logger.Error(
			"backend_shutdown",
			"error",
			err,
		)
	}
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		metrics.Default.Inc("recovery_http_requests_total", map[string]string{"method": c.Request.Method, "route": route, "status": http.StatusText(c.Writer.Status())})
		metrics.Default.Observe("recovery_http_request", map[string]string{"route": route}, time.Since(start))

		logger.Info(
			"http_request",
			"method",
			c.Request.Method,
			"route", route,
			"correlation_id", correlation.From(c.Request.Context()),
			"status",
			c.Writer.Status(),
			"duration_ms",
			time.Since(start).Milliseconds(),
		)
	}
}

func cors(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Origin") == origin {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Correlation-ID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

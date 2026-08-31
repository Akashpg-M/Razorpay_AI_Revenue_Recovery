package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"revenue-recovery/backend/internal/config"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/logging"
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
	router.Use(requestLogger(logger))

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
			checks["redis"] = "error"
			ready = false
		}

		if err := decisionClient.Health(checkCtx); err != nil {
			checks["decision_service"] = "error"
			ready = false
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

		logger.Info(
			"http_request",
			"method",
			c.Request.Method,
			"path",
			c.Request.URL.Path,
			"status",
			c.Writer.Status(),
			"duration_ms",
			time.Since(start).Milliseconds(),
		)
	}
}

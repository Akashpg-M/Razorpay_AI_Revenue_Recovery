package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"revenue-recovery/backend/internal/config"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/executor"
	"revenue-recovery/backend/internal/integrations/razorpay"
	"revenue-recovery/backend/internal/logging"
	"revenue-recovery/backend/internal/metrics"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/promises"
	"revenue-recovery/backend/internal/store"
)

type reassessor struct {
	decisions *decisioning.Service
	scheduler *orchestrator.Scheduler
}

func (r reassessor) Reassess(ctx context.Context, caseID domain.ID) error {
	snapshot, err := r.decisions.Decide(ctx, caseID)
	if err != nil {
		return err
	}
	_, err = r.scheduler.Schedule(ctx, snapshot)
	return err
}

func main() {
	cfg := config.Load()
	logger := logging.New()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("worker_database_initialization_failed", "error", err)
		return
	}
	defer db.Close()
	repository := store.NewPostgres(db)
	decisionClient := decisionclient.New(cfg.DecisionServiceURL)
	contextService := recoverycontext.NewService(repository)
	decisionService := decisioning.NewService(contextService, decisionClient, repository)
	scheduler := orchestrator.NewScheduler(repository)

	razorpayClient := razorpay.NewClient(cfg.RazorpayAPIURL, cfg.RazorpayKeyID, cfg.RazorpayKeySecret)
	paymentLinks := razorpay.NewPaymentLinkExecutor(razorpayClient, repository)
	registry := executor.NewRegistry(
		executor.NewEmailExecutor(repository),
		executor.NewPaymentLinkExecutor(paymentLinks),
		executor.NewRetryExecutor(repository),
	)
	hostname, _ := os.Hostname()
	worker := orchestrator.NewWorker(repository, contextService, registry, "worker:"+hostname)
	reassessment := reassessor{decisions: decisionService, scheduler: scheduler}
	worker.SetReassessor(reassessment)
	promiseService := promises.NewService(repository, reassessment)
	health := http.NewServeMux()
	health.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"worker","status":"ok"}`))
	})
	health.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		var schema string
		if err := db.QueryRow(r.Context(), `SELECT value FROM platform_metadata WHERE key='schema_version'`).Scan(&schema); err != nil || schema != "phase_34" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"service":"worker","status":"not_ready"}`))
			return
		}
		_, _ = w.Write([]byte(`{"service":"worker","status":"ready"}`))
	})
	health.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(metrics.Default.Prometheus()))
	})
	server := &http.Server{Addr: ":" + cfg.WorkerHealthPort, Handler: health, ReadHeaderTimeout: 2 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("worker_health_server_failed", "error", err)
		}
	}()
	defer server.Shutdown(context.Background())

	logger.Info("durable_worker_started", "poll_interval", "1s")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("durable_worker_stopped")
			return
		case <-ticker.C:
			started := time.Now()
			if err := promiseService.RunDueCheck(ctx, "promise-worker:"+hostname); err != nil && !errors.Is(err, promises.ErrNoDuePromise) {
				logger.Log(ctx, slog.LevelError, "promise_check_processing_failed", "error", err)
			}
			if _, err := worker.RunOnce(ctx); err != nil && !errors.Is(err, orchestrator.ErrNoDueWork) {
				metrics.Default.Inc("recovery_worker_failures_total", map[string]string{"loop": "scheduled_action"})
				logger.Log(ctx, slog.LevelError, "scheduled_action_processing_failed", "error", err)
			}
			metrics.Default.Observe("recovery_worker_loop", nil, time.Since(started))
			if err := worker.RunObservationOnce(ctx); err != nil && !errors.Is(err, orchestrator.ErrNoDueObservation) {
				logger.Log(ctx, slog.LevelError, "outcome_observation_processing_failed", "error", err)
			}
		}
	}
}

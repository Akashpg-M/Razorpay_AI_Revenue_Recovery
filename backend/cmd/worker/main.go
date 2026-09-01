package main

import (
	"context"
	"errors"
	"log/slog"
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

	logger.Info("durable_worker_started", "poll_interval", "1s")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("durable_worker_stopped")
			return
		case <-ticker.C:
			if err := promiseService.RunDueCheck(ctx, "promise-worker:"+hostname); err != nil && !errors.Is(err, promises.ErrNoDuePromise) {
				logger.Log(ctx, slog.LevelError, "promise_check_processing_failed", "error", err)
			}
			if _, err := worker.RunOnce(ctx); err != nil && !errors.Is(err, orchestrator.ErrNoDueWork) {
				logger.Log(ctx, slog.LevelError, "scheduled_action_processing_failed", "error", err)
			}
			if err := worker.RunObservationOnce(ctx); err != nil && !errors.Is(err, orchestrator.ErrNoDueObservation) {
				logger.Log(ctx, slog.LevelError, "outcome_observation_processing_failed", "error", err)
			}
		}
	}
}

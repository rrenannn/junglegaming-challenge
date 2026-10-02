package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/application/service"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

const (
	pendingReferencePollInterval = 5 * time.Second
	pendingReferenceBatchSize    = 20
)

// PendingReferenceWorker periodically retries REFUND/ROLLBACK operations
// whose reference wasn't found yet. It never touches SQS or HTTP directly —
// only Postgres (to find due transactions) and ProcessWagerService (to
// resolve them), so it lives outside internal/adapter/sqs.
type PendingReferenceWorker struct {
	uow     repository.UnitOfWork
	process *service.ProcessWagerService
	logger  *slog.Logger
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewPendingReferenceWorker(uow repository.UnitOfWork, process *service.ProcessWagerService, logger *slog.Logger) *PendingReferenceWorker {
	return &PendingReferenceWorker{uow: uow, process: process, logger: logger}
}

func (w *PendingReferenceWorker) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})

	w.logger.Info("pending reference worker starting")
	go w.run(runCtx)
	return nil
}

func (w *PendingReferenceWorker) Shutdown(ctx context.Context) error {
	if w.cancel == nil {
		return nil
	}
	w.logger.Info("pending reference worker shutting down")
	w.cancel()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *PendingReferenceWorker) run(ctx context.Context) {
	defer close(w.done)

	ticker := time.NewTicker(pendingReferencePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runBatch(ctx)
		}
	}
}

func (w *PendingReferenceWorker) runBatch(ctx context.Context) {
	var due []*domain.WagerTransaction
	err := w.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		var err error
		due, err = repos.Wagers().FindDuePendingReferences(ctx, pendingReferenceBatchSize)
		return err
	})
	if err != nil {
		w.logger.Error("find due pending references", "error", err)
		return
	}

	for _, tx := range due {
		result, err := w.process.ResolvePendingReference(ctx, tx.ID())
		if err != nil {
			w.logger.Error("resolve pending reference", "transactionId", tx.ID(), "error", err)
			continue
		}
		w.logger.Info("pending reference resolved", "transactionId", tx.ID(), "status", result.Status, "alreadyProcessed", result.AlreadyProcessed)
	}
}

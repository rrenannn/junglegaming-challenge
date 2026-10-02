package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
)

func seedOutboxEvent(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	uow := postgres.NewUnitOfWork(pool)
	event := domain.Event{
		ID:          uuid.NewString(),
		Type:        domain.EventWalletBalanceChanged,
		AggregateID: uuid.NewString(),
		OccurredAt:  time.Now(),
		Version:     1,
		Data:        map[string]string{"test": "true"},
	}
	err := uow.WithinTransaction(context.Background(), func(ctx context.Context, repos repository.TransactionRepositories) error {
		return repos.Outbox().Create(ctx, event)
	})
	if err != nil {
		t.Fatalf("seed outbox event: %v", err)
	}
	return event.ID
}

func claimBatch(t *testing.T, pool *pgxpool.Pool, workerID string, lease time.Duration, limit int) []*repository.OutboxEvent {
	t.Helper()
	uow := postgres.NewUnitOfWork(pool)
	var claimed []*repository.OutboxEvent
	err := uow.WithinTransaction(context.Background(), func(ctx context.Context, repos repository.TransactionRepositories) error {
		var err error
		claimed, err = repos.Outbox().ClaimBatch(ctx, workerID, lease, limit)
		return err
	})
	if err != nil {
		t.Fatalf("claim batch: %v", err)
	}
	return claimed
}

func TestOutboxRepository_SkipLockedPreventsDoubleClaim(t *testing.T) {
	pool := newTestPool(t)
	seedOutboxEvent(t, pool)

	claimedA := claimBatch(t, pool, "worker-a", time.Minute, 10)
	if len(claimedA) != 1 {
		t.Fatalf("worker-a claimed %d events, want 1", len(claimedA))
	}

	claimedB := claimBatch(t, pool, "worker-b", time.Minute, 10)
	if len(claimedB) != 0 {
		t.Fatalf("worker-b claimed %d events, want 0 (lease still held by worker-a)", len(claimedB))
	}
}

func TestOutboxRepository_ExpiredLeaseIsReclaimed(t *testing.T) {
	pool := newTestPool(t)
	eventID := seedOutboxEvent(t, pool)

	claimedA := claimBatch(t, pool, "worker-a", time.Minute, 10)
	if len(claimedA) != 1 {
		t.Fatalf("worker-a claimed %d events, want 1", len(claimedA))
	}

	if _, err := pool.Exec(context.Background(), `UPDATE outbox_events SET locked_until = now() - interval '1 minute' WHERE id = $1`, eventID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	claimedB := claimBatch(t, pool, "worker-b", time.Minute, 10)
	if len(claimedB) != 1 || claimedB[0].ID != eventID {
		t.Fatalf("worker-b should reclaim the event with an expired lease, got %+v", claimedB)
	}
}

func TestOutboxRepository_PublishedEventIsNeverReclaimed(t *testing.T) {
	pool := newTestPool(t)
	eventID := seedOutboxEvent(t, pool)

	claimed := claimBatch(t, pool, "worker-a", time.Minute, 10)
	if len(claimed) != 1 {
		t.Fatalf("claimed %d events, want 1", len(claimed))
	}

	uow := postgres.NewUnitOfWork(pool)
	err := uow.WithinTransaction(context.Background(), func(ctx context.Context, repos repository.TransactionRepositories) error {
		return repos.Outbox().MarkPublished(ctx, eventID, time.Now())
	})
	if err != nil {
		t.Fatalf("mark published: %v", err)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE outbox_events SET locked_until = now() - interval '1 minute' WHERE id = $1`, eventID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	claimedAgain := claimBatch(t, pool, "worker-b", time.Minute, 10)
	if len(claimedAgain) != 0 {
		t.Fatalf("a published event should never be reclaimed, got %+v", claimedAgain)
	}
}

func TestOutboxRepository_ReleaseForRetryReschedulesNextAttempt(t *testing.T) {
	pool := newTestPool(t)
	eventID := seedOutboxEvent(t, pool)

	claimed := claimBatch(t, pool, "worker-a", time.Minute, 10)
	if len(claimed) != 1 {
		t.Fatalf("claimed %d events, want 1", len(claimed))
	}

	future := time.Now().Add(time.Hour)
	uow := postgres.NewUnitOfWork(pool)
	err := uow.WithinTransaction(context.Background(), func(ctx context.Context, repos repository.TransactionRepositories) error {
		return repos.Outbox().ReleaseForRetry(ctx, eventID, future)
	})
	if err != nil {
		t.Fatalf("release for retry: %v", err)
	}

	tooEarly := claimBatch(t, pool, "worker-b", time.Minute, 10)
	if len(tooEarly) != 0 {
		t.Fatalf("event should not be claimable before its rescheduled next_attempt_at, got %+v", tooEarly)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE outbox_events SET next_attempt_at = now() - interval '1 minute' WHERE id = $1`, eventID); err != nil {
		t.Fatalf("backdate next_attempt_at: %v", err)
	}

	readyNow := claimBatch(t, pool, "worker-b", time.Minute, 10)
	if len(readyNow) != 1 || readyNow[0].ID != eventID {
		t.Fatalf("event should be claimable once next_attempt_at is due, got %+v", readyNow)
	}
}

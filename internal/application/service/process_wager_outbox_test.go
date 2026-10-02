package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

func outboxEventTypes(t *testing.T, pool *pgxpool.Pool, aggregateID string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT event_type FROM outbox_events WHERE aggregate_id = $1 ORDER BY occurred_at`, aggregateID)
	if err != nil {
		t.Fatalf("query outbox events: %v", err)
	}
	defer rows.Close()

	var types []string
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			t.Fatalf("scan outbox event: %v", err)
		}
		types = append(types, eventType)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate outbox events: %v", err)
	}
	return types
}

func countUnpublishedOutboxEvents(t *testing.T, pool *pgxpool.Pool, aggregateID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox_events WHERE aggregate_id = $1 AND published_at IS NULL`, aggregateID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count unpublished outbox events: %v", err)
	}
	return count
}

func TestProcessWagerService_ProcessedBetEmitsTwoEvents(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-outbox-bet", "100.00")

	if _, err := svc.Execute(context.Background(), betCommand(walletID, "player-outbox-bet", "30.00")); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	types := outboxEventTypes(t, pool, walletID)
	want := []string{string(domain.EventWagerTransactionProcessed), string(domain.EventWalletBalanceChanged)}
	if len(types) != len(want) || types[0] != want[0] || types[1] != want[1] {
		t.Fatalf("outbox event types = %v, want %v", types, want)
	}
	if count := countUnpublishedOutboxEvents(t, pool, walletID); count != 2 {
		t.Fatalf("unpublished outbox events = %d, want 2", count)
	}
}

func TestProcessWagerService_RejectedBetEmitsOneEvent(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-outbox-rejected", "10.00")

	result, err := svc.Execute(context.Background(), betCommand(walletID, "player-outbox-rejected", "9999.00"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != domain.StatusRejected {
		t.Fatalf("status = %s, want REJECTED", result.Status)
	}

	types := outboxEventTypes(t, pool, walletID)
	want := []string{string(domain.EventWagerTransactionRejected)}
	if len(types) != len(want) || types[0] != want[0] {
		t.Fatalf("outbox event types = %v, want %v", types, want)
	}
}

func TestProcessWagerService_LossEmitsOnlyProcessedEvent(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-outbox-loss", "50.00")

	lossCmd := betCommand(walletID, "player-outbox-loss", "0.00")
	lossCmd.Kind = domain.KindLoss

	result, err := svc.Execute(context.Background(), lossCmd)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != domain.StatusProcessed {
		t.Fatalf("status = %s, want PROCESSED", result.Status)
	}

	types := outboxEventTypes(t, pool, walletID)
	want := []string{string(domain.EventWagerTransactionProcessed)}
	if len(types) != len(want) || types[0] != want[0] {
		t.Fatalf("outbox event types = %v, want %v (LOSS must not emit WalletBalanceChanged)", types, want)
	}
}

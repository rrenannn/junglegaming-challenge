package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

func TestProcessWagerService_RollbackBeforeReferenceGoesPending(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-pending-1", "100.00")

	rollbackCmd := betCommand(walletID, "player-pending-1", "30.00")
	rollbackCmd.Kind = domain.KindRollback
	rollbackCmd.ReferenceExternalTransactionID = uuid.NewString() // never submitted

	result, err := svc.Execute(context.Background(), rollbackCmd)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != domain.StatusPendingReference {
		t.Fatalf("Status = %s, want PENDING_REFERENCE", result.Status)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "100.00" {
		t.Fatalf("balance = %s, want 100.00 (pending reference must not move money)", balance)
	}
	if count := countLedgerEntries(t, pool, walletID); count != 0 {
		t.Fatalf("ledger entries = %d, want 0", count)
	}
}

func TestProcessWagerService_ResolvePendingReferenceOnceOriginalArrives(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-pending-2", "100.00")

	betExternalID := uuid.NewString()

	rollbackCmd := betCommand(walletID, "player-pending-2", "30.00")
	rollbackCmd.Kind = domain.KindRollback
	rollbackCmd.ReferenceExternalTransactionID = betExternalID

	pending, err := svc.Execute(context.Background(), rollbackCmd)
	if err != nil {
		t.Fatalf("rollback Execute: %v", err)
	}
	if pending.Status != domain.StatusPendingReference {
		t.Fatalf("Status = %s, want PENDING_REFERENCE", pending.Status)
	}

	betCmd := betCommand(walletID, "player-pending-2", "30.00")
	betCmd.ExternalTransactionID = betExternalID
	betResult, err := svc.Execute(context.Background(), betCmd)
	if err != nil || betResult.Status != domain.StatusProcessed {
		t.Fatalf("bet: result=%+v err=%v", betResult, err)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "70.00" {
		t.Fatalf("balance after bet = %s, want 70.00", balance)
	}

	resolved, err := svc.ResolvePendingReference(context.Background(), pending.TransactionID)
	if err != nil {
		t.Fatalf("ResolvePendingReference: %v", err)
	}
	if resolved.Status != domain.StatusProcessed {
		t.Fatalf("resolved status = %s, want PROCESSED", resolved.Status)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "100.00" {
		t.Fatalf("balance after rollback resolves = %s, want 100.00", balance)
	}
	if count := countLedgerEntries(t, pool, walletID); count != 2 {
		t.Fatalf("ledger entries = %d, want 2 (bet debit + rollback credit)", count)
	}
}

func TestProcessWagerService_ResolvePendingReferenceStillMissingReschedules(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-pending-3", "100.00")

	rollbackCmd := betCommand(walletID, "player-pending-3", "30.00")
	rollbackCmd.Kind = domain.KindRollback
	rollbackCmd.ReferenceExternalTransactionID = uuid.NewString()

	pending, err := svc.Execute(context.Background(), rollbackCmd)
	if err != nil {
		t.Fatalf("rollback Execute: %v", err)
	}

	resolved, err := svc.ResolvePendingReference(context.Background(), pending.TransactionID)
	if err != nil {
		t.Fatalf("ResolvePendingReference: %v", err)
	}
	if resolved.Status != domain.StatusPendingReference {
		t.Fatalf("status = %s, want still PENDING_REFERENCE", resolved.Status)
	}

	var attempts int
	err = pool.QueryRow(context.Background(), `SELECT attempts FROM wager_transactions WHERE id = $1`, pending.TransactionID).Scan(&attempts)
	if err != nil {
		t.Fatalf("query attempts: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestProcessWagerService_ResolvePendingReferenceExpiresAfterMaxAttempts(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-pending-4", "100.00")

	rollbackCmd := betCommand(walletID, "player-pending-4", "30.00")
	rollbackCmd.Kind = domain.KindRollback
	rollbackCmd.ReferenceExternalTransactionID = uuid.NewString()

	pending, err := svc.Execute(context.Background(), rollbackCmd)
	if err != nil {
		t.Fatalf("rollback Execute: %v", err)
	}

	// Simulate having already exhausted the retry budget, instead of
	// calling ResolvePendingReference maxPendingReferenceAttempts times.
	_, err = pool.Exec(context.Background(), `UPDATE wager_transactions SET attempts = $1 WHERE id = $2`, maxPendingReferenceAttempts, pending.TransactionID)
	if err != nil {
		t.Fatalf("set attempts: %v", err)
	}

	resolved, err := svc.ResolvePendingReference(context.Background(), pending.TransactionID)
	if err != nil {
		t.Fatalf("ResolvePendingReference: %v", err)
	}
	if resolved.Status != domain.StatusRejected {
		t.Fatalf("status = %s, want REJECTED", resolved.Status)
	}
	if resolved.FailureCode == nil || *resolved.FailureCode != domain.FailureReferenceNotFound {
		t.Fatalf("FailureCode = %v, want REFERENCE_NOT_FOUND", resolved.FailureCode)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "100.00" {
		t.Fatalf("balance = %s, want unchanged 100.00", balance)
	}
}

func TestProcessWagerService_ResolvePendingReferenceAlreadyResolvedIsNoOp(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-pending-5", "100.00")

	betCmd := betCommand(walletID, "player-pending-5", "30.00")
	betResult, err := svc.Execute(context.Background(), betCmd)
	if err != nil {
		t.Fatalf("bet Execute: %v", err)
	}

	resolved, err := svc.ResolvePendingReference(context.Background(), betResult.TransactionID)
	if err != nil {
		t.Fatalf("ResolvePendingReference: %v", err)
	}
	if !resolved.AlreadyProcessed {
		t.Fatal("resolving a non-pending transaction should be a safe no-op")
	}
	if resolved.Status != domain.StatusProcessed {
		t.Fatalf("status = %s, want PROCESSED (unchanged)", resolved.Status)
	}
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

func TestProcessWagerService_IdempotentReplayWithSameKeyAndContent(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-idem-1", "100.00")

	cmd := betCommand(walletID, "player-idem-1", "30.00")
	cmd.IdempotencyKey = cmd.ExternalTransactionID

	first, err := svc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if first.AlreadyProcessed {
		t.Fatal("first submission should not be marked as already processed")
	}

	second, err := svc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !second.AlreadyProcessed {
		t.Fatal("resubmission with the same idempotency key and content should be a replay")
	}
	if second.TransactionID != first.TransactionID {
		t.Fatalf("replay TransactionID = %s, want %s", second.TransactionID, first.TransactionID)
	}
	if second.Status != first.Status {
		t.Fatalf("replay Status = %s, want %s", second.Status, first.Status)
	}

	if count := countWagerTransactions(t, pool, walletID); count != 1 {
		t.Fatalf("wager transactions = %d, want 1", count)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "70.00" {
		t.Fatalf("final balance = %s, want 70.00 (resubmission must not debit twice)", balance)
	}
}

func TestProcessWagerService_IdempotentReplayWithDifferentContentConflicts(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-idem-2", "100.00")

	first := betCommand(walletID, "player-idem-2", "30.00")
	first.IdempotencyKey = first.ExternalTransactionID
	if _, err := svc.Execute(context.Background(), first); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	second := betCommand(walletID, "player-idem-2", "40.00")
	second.ExternalTransactionID = first.ExternalTransactionID
	second.IdempotencyKey = first.ExternalTransactionID

	_, err := svc.Execute(context.Background(), second)
	if !errors.Is(err, repository.ErrIdempotencyKeyConflict) {
		t.Fatalf("err = %v, want repository.ErrIdempotencyKeyConflict", err)
	}
}

func TestProcessWagerService_ReplayReturnsOriginalSnapshotAfterLaterMovements(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-idem-4", "100.00")

	betCmd := betCommand(walletID, "player-idem-4", "30.00")
	betCmd.IdempotencyKey = betCmd.ExternalTransactionID
	betResult, err := svc.Execute(context.Background(), betCmd)
	if err != nil {
		t.Fatalf("bet Execute: %v", err)
	}
	if betResult.BalanceAfter.Decimal() != "70.00" {
		t.Fatalf("bet balanceAfter = %s, want 70.00", betResult.BalanceAfter.Decimal())
	}

	winCmd := betCommand(walletID, "player-idem-4", "15.00")
	winCmd.Kind = domain.KindWin
	if _, err := svc.Execute(context.Background(), winCmd); err != nil {
		t.Fatalf("win Execute: %v", err)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "85.00" {
		t.Fatalf("balance after win = %s, want 85.00", balance)
	}

	replay, err := svc.Execute(context.Background(), betCmd)
	if err != nil {
		t.Fatalf("replay Execute: %v", err)
	}
	if !replay.AlreadyProcessed {
		t.Fatal("resubmitting the original bet should be a replay")
	}
	if replay.TransactionID != betResult.TransactionID {
		t.Fatalf("replay TransactionID = %s, want %s", replay.TransactionID, betResult.TransactionID)
	}
	if replay.BalanceAfter.Decimal() != "70.00" {
		t.Fatalf("replay balanceAfter = %s, want the original snapshot 70.00, not the current 85.00", replay.BalanceAfter.Decimal())
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "85.00" {
		t.Fatalf("balance after replay = %s, want unchanged 85.00", balance)
	}
	if count := countWagerTransactions(t, pool, walletID); count != 2 {
		t.Fatalf("wager transactions = %d, want 2 (bet + win, no extra row for the replay)", count)
	}
}

func TestProcessWagerService_IdempotencyKeyDefaultsToExternalTransactionID(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-idem-3", "100.00")

	cmd := betCommand(walletID, "player-idem-3", "10.00")
	cmd.IdempotencyKey = "" // left unset on purpose

	first, err := svc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	second, err := svc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !second.AlreadyProcessed || second.TransactionID != first.TransactionID {
		t.Fatalf("second = %+v, want a replay of %+v", second, first)
	}
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
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

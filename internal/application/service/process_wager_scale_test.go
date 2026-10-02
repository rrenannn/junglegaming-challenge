package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

// TestProcessWagerService_FiftyContestedBetsAgainstSameBalance generalizes
// TestProcessWagerService_TwoConcurrentBetsAgainstSameBalance to a much
// higher concurrency, proving the pessimistic wallet lock (SELECT ... FOR
// UPDATE) serializes correctly under real contention, not just n=2: a
// balance of 250.00 can only ever afford 25 of the 50 concurrent 10.00
// bets, no matter the interleaving.
func TestProcessWagerService_FiftyContestedBetsAgainstSameBalance(t *testing.T) {
	const (
		betCount   = 50
		betAmount  = "10.00"
		balance    = "250.00"
		wantProc   = 25
		wantReject = betCount - wantProc
	)

	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-scale-1", balance)

	cmds := make([]ProcessWagerCommand, betCount)
	for i := range cmds {
		cmds[i] = betCommand(walletID, "player-scale-1", betAmount)
	}

	results := make([]*ProcessWagerResult, betCount)
	errs := make([]error, betCount)
	var wg sync.WaitGroup
	wg.Add(betCount)
	for i := range cmds {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.Execute(context.Background(), cmds[i])
		}(i)
	}
	wg.Wait()

	processed, rejected := 0, 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Execute(%d): unexpected error %v", i, err)
		}
		switch results[i].Status {
		case domain.StatusProcessed:
			processed++
		case domain.StatusRejected:
			rejected++
			if results[i].FailureCode == nil || *results[i].FailureCode != domain.FailureInsufficientFunds {
				t.Fatalf("rejected result %d FailureCode = %v, want INSUFFICIENT_FUNDS", i, results[i].FailureCode)
			}
		default:
			t.Fatalf("result %d status = %s, want PROCESSED or REJECTED", i, results[i].Status)
		}
	}

	if processed != wantProc || rejected != wantReject {
		t.Fatalf("processed=%d rejected=%d, want %d and %d", processed, rejected, wantProc, wantReject)
	}
	if finalBalance := fetchWalletBalance(t, pool, walletID); finalBalance != "0.00" {
		t.Fatalf("final balance = %s, want 0.00", finalBalance)
	}
	if count := countLedgerEntries(t, pool, walletID); count != wantProc {
		t.Fatalf("ledger entries = %d, want %d", count, wantProc)
	}
}

// TestProcessWagerService_ParallelismAcrossWallets proves that concurrent
// operations on different wallets never interfere with each other — the
// pessimistic lock is per-row, not a global serialization point.
func TestProcessWagerService_ParallelismAcrossWallets(t *testing.T) {
	const walletCount = 10

	svc, pool := newTestService(t)
	playerIDs := make([]string, walletCount)
	walletIDs := make([]string, walletCount)
	for i := range walletIDs {
		playerIDs[i] = uuid.NewString()
		walletIDs[i] = seedWallet(t, pool, playerIDs[i], "100.00")
	}

	results := make([]*ProcessWagerResult, walletCount)
	errs := make([]error, walletCount)
	var wg sync.WaitGroup
	wg.Add(walletCount)
	for i, walletID := range walletIDs {
		go func(i int, walletID string) {
			defer wg.Done()
			results[i], errs[i] = svc.Execute(context.Background(), betCommand(walletID, playerIDs[i], "30.00"))
		}(i, walletID)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Execute(%d): unexpected error %v", i, err)
		}
		if results[i].Status != domain.StatusProcessed {
			t.Fatalf("result %d status = %s, want PROCESSED", i, results[i].Status)
		}
	}
	for i, walletID := range walletIDs {
		if balance := fetchWalletBalance(t, pool, walletID); balance != "70.00" {
			t.Fatalf("wallet %d balance = %s, want 70.00", i, balance)
		}
	}
}

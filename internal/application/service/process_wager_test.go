package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/clock"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/idgen"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
)

func newTestService(t *testing.T) (*ProcessWagerService, *pgxpool.Pool) {
	t.Helper()
	pool := newTestPool(t)
	uow := postgres.NewUnitOfWork(pool)
	return NewProcessWagerService(uow, clock.NewSystemClock(), idgen.NewUUIDGenerator(), port.NoopMetrics{}), pool
}

func seedWallet(t *testing.T, pool *pgxpool.Pool, playerID, decimalBalance string) string {
	t.Helper()
	balance, err := domain.ParseMoney(decimalBalance, domain.BRL)
	if err != nil {
		t.Fatalf("ParseMoney: %v", err)
	}
	walletID := uuid.NewString()
	wallet, err := domain.NewWallet(walletID, playerID, balance, time.Now())
	if err != nil {
		t.Fatalf("NewWallet: %v", err)
	}

	_, err = pool.Exec(context.Background(), `
		INSERT INTO wallets (id, player_id, currency, balance_minor, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, wallet.ID(), wallet.PlayerID(), string(wallet.Currency()), wallet.Balance().AmountMinor(),
		wallet.Version(), wallet.CreatedAt(), wallet.UpdatedAt())
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	return walletID
}

func fetchWalletBalance(t *testing.T, pool *pgxpool.Pool, walletID string) string {
	t.Helper()
	var balanceMinor int64
	err := pool.QueryRow(context.Background(), `SELECT balance_minor FROM wallets WHERE id = $1`, walletID).Scan(&balanceMinor)
	if err != nil {
		t.Fatalf("fetch wallet balance: %v", err)
	}
	money, err := domain.NewMoney(balanceMinor, domain.BRL)
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	return money.Decimal()
}

func countLedgerEntries(t *testing.T, pool *pgxpool.Pool, walletID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wallet_ledger_entries WHERE wallet_id = $1`, walletID).Scan(&count)
	if err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	return count
}

func insertConflictingTransaction(t *testing.T, pool *pgxpool.Pool, transactionID, walletID string) {
	t.Helper()
	now := time.Now()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO wager_transactions (
			id, wallet_id, provider_id, player_id, round_id, game_id,
			kind, status, direction, amount_minor, currency, created_at, updated_at
		) VALUES ($1,$2,'provider-a','player-x','round-x','game-x','BET','PENDING','NONE',100,'BRL',$3,$4)
	`, transactionID, walletID, now, now)
	if err != nil {
		t.Fatalf("insert conflicting transaction: %v", err)
	}
}

func betCommand(walletID, playerID, decimalAmount string) ProcessWagerCommand {
	amount, _ := domain.ParseMoney(decimalAmount, domain.BRL)
	return ProcessWagerCommand{
		TransactionID:         uuid.NewString(),
		ProviderID:            "provider-a",
		PlayerID:              playerID,
		WalletID:              walletID,
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  domain.KindBet,
		Amount:                amount,
		ExternalTransactionID: uuid.NewString(),
	}
}

func TestProcessWagerService_TwoConcurrentBetsAgainstSameBalance(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-1", "100.00")

	cmdA := betCommand(walletID, "player-1", "80.00")
	cmdB := betCommand(walletID, "player-1", "80.00")

	var wg sync.WaitGroup
	results := make([]*ProcessWagerResult, 2)
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0], errs[0] = svc.Execute(context.Background(), cmdA)
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = svc.Execute(context.Background(), cmdB)
	}()
	wg.Wait()

	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("unexpected errors: %v, %v", errs[0], errs[1])
	}

	processedCount, rejectedCount := 0, 0
	for _, result := range results {
		switch result.Status {
		case domain.StatusProcessed:
			processedCount++
		case domain.StatusRejected:
			rejectedCount++
			if result.FailureCode == nil || *result.FailureCode != domain.FailureInsufficientFunds {
				t.Fatalf("rejected result FailureCode = %v, want INSUFFICIENT_FUNDS", result.FailureCode)
			}
		}
	}
	if processedCount != 1 || rejectedCount != 1 {
		t.Fatalf("processed=%d rejected=%d, want 1 and 1", processedCount, rejectedCount)
	}

	if balance := fetchWalletBalance(t, pool, walletID); balance != "20.00" {
		t.Fatalf("final balance = %s, want 20.00", balance)
	}
	if count := countLedgerEntries(t, pool, walletID); count != 1 {
		t.Fatalf("ledger entries = %d, want 1", count)
	}
}

func TestProcessWagerService_FullRollbackOnLaterStepFailure(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-2", "100.00")

	cmd := betCommand(walletID, "player-2", "30.00")
	// Force the service's own insert to collide on the primary key so the
	// unit of work rolls back after the wallet was already mutated in memory.
	insertConflictingTransaction(t, pool, cmd.TransactionID, walletID)

	if _, err := svc.Execute(context.Background(), cmd); err == nil {
		t.Fatal("expected error due to primary key conflict")
	}

	if balance := fetchWalletBalance(t, pool, walletID); balance != "100.00" {
		t.Fatalf("final balance = %s, want 100.00 (transaction must have rolled back)", balance)
	}
}

func TestProcessWagerService_BetThenWin(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-3", "50.00")

	betResult, err := svc.Execute(context.Background(), betCommand(walletID, "player-3", "20.00"))
	if err != nil || betResult.Status != domain.StatusProcessed {
		t.Fatalf("bet: result=%+v err=%v", betResult, err)
	}

	winCmd := betCommand(walletID, "player-3", "15.00")
	winCmd.Kind = domain.KindWin
	winResult, err := svc.Execute(context.Background(), winCmd)
	if err != nil || winResult.Status != domain.StatusProcessed {
		t.Fatalf("win: result=%+v err=%v", winResult, err)
	}
	if winResult.BalanceAfter.Decimal() != "45.00" {
		t.Fatalf("balance after win = %s, want 45.00", winResult.BalanceAfter.Decimal())
	}
	if count := countLedgerEntries(t, pool, walletID); count != 2 {
		t.Fatalf("ledger entries = %d, want 2", count)
	}
}

func TestProcessWagerService_GeneratesTransactionIDWhenAbsent(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-generated-id", "50.00")

	cmd := betCommand(walletID, "player-generated-id", "20.00")
	cmd.TransactionID = ""

	result, err := svc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.TransactionID == "" {
		t.Fatal("expected a generated transaction id, got empty string")
	}
	if _, err := uuid.Parse(result.TransactionID); err != nil {
		t.Fatalf("generated transaction id %q is not a valid UUID: %v", result.TransactionID, err)
	}
}

func TestProcessWagerService_RefundThenRollbackRejected(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-4", "100.00")

	betCmd := betCommand(walletID, "player-4", "80.00")
	betResult, err := svc.Execute(context.Background(), betCmd)
	if err != nil || betResult.Status != domain.StatusProcessed {
		t.Fatalf("bet: result=%+v err=%v", betResult, err)
	}

	refundCmd := betCommand(walletID, "player-4", "80.00")
	refundCmd.Kind = domain.KindRefund
	refundCmd.ReferenceExternalTransactionID = betCmd.ExternalTransactionID
	refundResult, err := svc.Execute(context.Background(), refundCmd)
	if err != nil || refundResult.Status != domain.StatusProcessed {
		t.Fatalf("refund: result=%+v err=%v", refundResult, err)
	}
	if balance := fetchWalletBalance(t, pool, walletID); balance != "100.00" {
		t.Fatalf("balance after refund = %s, want 100.00", balance)
	}

	rollbackCmd := betCommand(walletID, "player-4", "80.00")
	rollbackCmd.Kind = domain.KindRollback
	rollbackCmd.ReferenceExternalTransactionID = betCmd.ExternalTransactionID
	rollbackResult, err := svc.Execute(context.Background(), rollbackCmd)
	if err != nil {
		t.Fatalf("rollback against already-reversed bet: unexpected service error %v", err)
	}
	if rollbackResult.Status != domain.StatusRejected {
		t.Fatalf("rollback status = %s, want REJECTED", rollbackResult.Status)
	}
	if rollbackResult.FailureCode == nil || *rollbackResult.FailureCode != domain.FailureAlreadyReversed {
		t.Fatalf("rollback FailureCode = %v, want ALREADY_REVERSED", rollbackResult.FailureCode)
	}
}

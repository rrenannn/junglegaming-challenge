package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/clock"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/idgen"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
)

func newTestOpenWalletService(t *testing.T) (*OpenWalletService, *pgxpool.Pool) {
	t.Helper()
	pool := newTestPool(t)
	uow := postgres.NewUnitOfWork(pool)
	return NewOpenWalletService(uow, clock.NewSystemClock(), idgen.NewUUIDGenerator()), pool
}

func countWagerTransactions(t *testing.T, pool *pgxpool.Pool, walletID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wager_transactions WHERE wallet_id = $1`, walletID).Scan(&count)
	if err != nil {
		t.Fatalf("count wager transactions: %v", err)
	}
	return count
}

func TestOpenWalletService_PositiveOpeningBalanceCreatesOpeningTransactionAndLedger(t *testing.T) {
	svc, pool := newTestOpenWalletService(t)

	wallet, err := svc.Execute(context.Background(), OpenWalletCommand{
		PlayerID:       "player-1",
		Currency:       "BRL",
		OpeningBalance: "100.00",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if wallet.Balance().Decimal() != "100.00" {
		t.Fatalf("balance = %s, want 100.00", wallet.Balance().Decimal())
	}
	if wallet.Version() != 1 {
		t.Fatalf("version = %d, want 1", wallet.Version())
	}
	if count := countWagerTransactions(t, pool, wallet.ID()); count != 1 {
		t.Fatalf("wager transactions = %d, want 1", count)
	}
	if count := countLedgerEntries(t, pool, wallet.ID()); count != 1 {
		t.Fatalf("ledger entries = %d, want 1", count)
	}
}

func TestOpenWalletService_ZeroOpeningBalanceCreatesNoTransactionOrLedger(t *testing.T) {
	svc, pool := newTestOpenWalletService(t)

	wallet, err := svc.Execute(context.Background(), OpenWalletCommand{
		PlayerID: "player-2",
		Currency: "BRL",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !wallet.Balance().IsZero() {
		t.Fatalf("balance = %s, want 0.00", wallet.Balance().Decimal())
	}
	if count := countWagerTransactions(t, pool, wallet.ID()); count != 0 {
		t.Fatalf("wager transactions = %d, want 0", count)
	}
	if count := countLedgerEntries(t, pool, wallet.ID()); count != 0 {
		t.Fatalf("ledger entries = %d, want 0", count)
	}
}

func TestOpenWalletService_DuplicatePlayerAndCurrencyIsRejected(t *testing.T) {
	svc, _ := newTestOpenWalletService(t)
	cmd := OpenWalletCommand{PlayerID: "player-3", Currency: "BRL", OpeningBalance: "10.00"}

	if _, err := svc.Execute(context.Background(), cmd); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	_, err := svc.Execute(context.Background(), cmd)
	if !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("second Execute error = %v, want repository.ErrAlreadyExists", err)
	}
}

func TestOpenWalletService_InvalidCurrencyIsRejected(t *testing.T) {
	svc, _ := newTestOpenWalletService(t)

	_, err := svc.Execute(context.Background(), OpenWalletCommand{PlayerID: "player-4", Currency: "XYZ"})
	if !domain.HasFailureCode(err, domain.FailureInvalidCurrency) {
		t.Fatalf("err = %v, want FailureInvalidCurrency", err)
	}
}

func TestOpenWalletService_InvalidOpeningBalanceIsRejected(t *testing.T) {
	svc, _ := newTestOpenWalletService(t)

	_, err := svc.Execute(context.Background(), OpenWalletCommand{PlayerID: "player-5", Currency: "BRL", OpeningBalance: "-10.00"})
	if !domain.HasFailureCode(err, domain.FailureInvalidAmount) {
		t.Fatalf("err = %v, want FailureInvalidAmount", err)
	}
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/clock"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/idgen"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
)

func TestReconcileWalletService_BalancedAfterBet(t *testing.T) {
	wagerSvc, pool := newTestService(t)
	uow := postgres.NewUnitOfWork(pool)
	openSvc := NewOpenWalletService(uow, clock.NewSystemClock(), idgen.NewUUIDGenerator())

	wallet, err := openSvc.Execute(context.Background(), OpenWalletCommand{
		PlayerID:       "player-reconcile-1",
		Currency:       "BRL",
		OpeningBalance: "100.00",
	})
	if err != nil {
		t.Fatalf("open wallet: %v", err)
	}

	if _, err := wagerSvc.Execute(context.Background(), betCommand(wallet.ID(), "player-reconcile-1", "30.00")); err != nil {
		t.Fatalf("seed bet: %v", err)
	}

	svc := NewReconcileWalletService(uow)
	result, err := svc.Execute(context.Background(), wallet.ID())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !result.Balanced {
		t.Fatalf("Balanced = false, want true (divergence=%v)", result.Divergence)
	}
	if result.Divergence != nil {
		t.Fatalf("Divergence = %v, want nil", result.Divergence)
	}
	if result.RecordedBalance.Decimal() != "70.00" || result.ComputedBalance.Decimal() != "70.00" {
		t.Fatalf("recorded=%s computed=%s, want both 70.00", result.RecordedBalance.Decimal(), result.ComputedBalance.Decimal())
	}
}

func TestReconcileWalletService_BalancedWithNoEntries(t *testing.T) {
	pool := newTestPool(t)
	uow := postgres.NewUnitOfWork(pool)
	openSvc := NewOpenWalletService(uow, clock.NewSystemClock(), idgen.NewUUIDGenerator())

	wallet, err := openSvc.Execute(context.Background(), OpenWalletCommand{
		PlayerID: "player-reconcile-2",
		Currency: "BRL",
	})
	if err != nil {
		t.Fatalf("open wallet: %v", err)
	}

	svc := NewReconcileWalletService(uow)
	result, err := svc.Execute(context.Background(), wallet.ID())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !result.Balanced {
		t.Fatalf("Balanced = false, want true (divergence=%v)", result.Divergence)
	}
	if !result.RecordedBalance.IsZero() || !result.ComputedBalance.IsZero() {
		t.Fatalf("recorded=%s computed=%s, want both zero", result.RecordedBalance.Decimal(), result.ComputedBalance.Decimal())
	}
}

func TestReconcileWalletService_DetectsDivergence(t *testing.T) {
	pool := newTestPool(t)
	uow := postgres.NewUnitOfWork(pool)
	openSvc := NewOpenWalletService(uow, clock.NewSystemClock(), idgen.NewUUIDGenerator())

	wallet, err := openSvc.Execute(context.Background(), OpenWalletCommand{
		PlayerID:       "player-reconcile-3",
		Currency:       "BRL",
		OpeningBalance: "100.00",
	})
	if err != nil {
		t.Fatalf("open wallet: %v", err)
	}

	// Simulates data corruption: directly tamper with the stored balance,
	// bypassing the domain, since there is no legitimate way to cause a
	// divergence through the public API.
	if _, err := pool.Exec(context.Background(), `UPDATE wallets SET balance_minor = 9000 WHERE id = $1`, wallet.ID()); err != nil {
		t.Fatalf("tamper wallet balance: %v", err)
	}

	svc := NewReconcileWalletService(uow)
	result, err := svc.Execute(context.Background(), wallet.ID())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.Balanced {
		t.Fatal("Balanced = true, want false")
	}
	if result.RecordedBalance.Decimal() != "90.00" {
		t.Fatalf("recorded = %s, want 90.00", result.RecordedBalance.Decimal())
	}
	if result.ComputedBalance.Decimal() != "100.00" {
		t.Fatalf("computed = %s, want 100.00", result.ComputedBalance.Decimal())
	}
	if result.Divergence == nil || result.Divergence.Decimal() != "-10.00" {
		t.Fatalf("divergence = %v, want -10.00", result.Divergence)
	}
}

func TestReconcileWalletService_NotFound(t *testing.T) {
	pool := newTestPool(t)
	svc := NewReconcileWalletService(postgres.NewUnitOfWork(pool))

	_, err := svc.Execute(context.Background(), uuid.NewString())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want repository.ErrNotFound", err)
	}
}

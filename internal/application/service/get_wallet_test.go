package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
)

func TestGetWalletService_ReturnsExistingWallet(t *testing.T) {
	pool := newTestPool(t)
	walletID := seedWallet(t, pool, "player-1", "42.50")

	svc := NewGetWalletService(postgres.NewUnitOfWork(pool))
	wallet, err := svc.Execute(context.Background(), walletID)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if wallet.ID() != walletID {
		t.Fatalf("ID = %s, want %s", wallet.ID(), walletID)
	}
	if wallet.Balance().Decimal() != "42.50" {
		t.Fatalf("balance = %s, want 42.50", wallet.Balance().Decimal())
	}
}

func TestGetWalletService_NotFound(t *testing.T) {
	pool := newTestPool(t)
	svc := NewGetWalletService(postgres.NewUnitOfWork(pool))

	_, err := svc.Execute(context.Background(), uuid.NewString())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want repository.ErrNotFound", err)
	}
}

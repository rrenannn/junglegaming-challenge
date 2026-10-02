package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
)

func TestGetTransactionService_OwnerProviderCanRead(t *testing.T) {
	wagerSvc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-tx-1", "100.00")

	result, err := wagerSvc.Execute(context.Background(), betCommand(walletID, "player-tx-1", "20.00"))
	if err != nil {
		t.Fatalf("seed bet: %v", err)
	}

	getSvc := NewGetTransactionService(postgres.NewUnitOfWork(pool))
	tx, err := getSvc.Execute(context.Background(), GetTransactionQuery{
		TransactionID:       result.TransactionID,
		RequesterProviderID: "provider-a",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if tx.ID() != result.TransactionID {
		t.Fatalf("ID = %s, want %s", tx.ID(), result.TransactionID)
	}
}

func TestGetTransactionService_OtherProviderIsRejected(t *testing.T) {
	wagerSvc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-tx-2", "100.00")

	result, err := wagerSvc.Execute(context.Background(), betCommand(walletID, "player-tx-2", "20.00"))
	if err != nil {
		t.Fatalf("seed bet: %v", err)
	}

	getSvc := NewGetTransactionService(postgres.NewUnitOfWork(pool))
	_, err = getSvc.Execute(context.Background(), GetTransactionQuery{
		TransactionID:       result.TransactionID,
		RequesterProviderID: "provider-b",
	})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want repository.ErrNotFound", err)
	}
}

func TestGetTransactionService_InternalCanReadAnyProvider(t *testing.T) {
	wagerSvc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-tx-3", "100.00")

	result, err := wagerSvc.Execute(context.Background(), betCommand(walletID, "player-tx-3", "20.00"))
	if err != nil {
		t.Fatalf("seed bet: %v", err)
	}

	getSvc := NewGetTransactionService(postgres.NewUnitOfWork(pool))
	tx, err := getSvc.Execute(context.Background(), GetTransactionQuery{TransactionID: result.TransactionID})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if tx.ID() != result.TransactionID {
		t.Fatalf("ID = %s, want %s", tx.ID(), result.TransactionID)
	}
}

func TestGetTransactionService_NotFound(t *testing.T) {
	pool := newTestPool(t)
	getSvc := NewGetTransactionService(postgres.NewUnitOfWork(pool))

	_, err := getSvc.Execute(context.Background(), GetTransactionQuery{TransactionID: uuid.NewString()})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want repository.ErrNotFound", err)
	}
}

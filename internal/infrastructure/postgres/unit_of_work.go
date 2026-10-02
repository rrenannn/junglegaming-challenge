package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	apprepository "github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	pgrepository "github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres/repository"
)

type UnitOfWork struct {
	pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

func (u *UnitOfWork) WithinTransaction(ctx context.Context, fn func(context.Context, apprepository.TransactionRepositories) error) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	repos := &transactionRepositories{
		wallets: pgrepository.NewWalletRepository(tx),
		wagers:  pgrepository.NewWagerTransactionRepository(tx),
		ledger:  pgrepository.NewLedgerRepository(tx),
	}

	if err := fn(ctx, repos); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

type transactionRepositories struct {
	wallets *pgrepository.WalletRepository
	wagers  *pgrepository.WagerTransactionRepository
	ledger  *pgrepository.LedgerRepository
}

func (r *transactionRepositories) Wallets() apprepository.WalletRepository {
	return r.wallets
}

func (r *transactionRepositories) Wagers() apprepository.WagerTransactionRepository {
	return r.wagers
}

func (r *transactionRepositories) Ledger() apprepository.LedgerRepository {
	return r.ledger
}

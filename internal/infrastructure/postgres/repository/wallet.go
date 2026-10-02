package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

const walletPlayerCurrencyUniqueConstraint = "wallets_player_currency_unique"

type WalletRepository struct {
	tx pgx.Tx
}

func NewWalletRepository(tx pgx.Tx) *WalletRepository {
	return &WalletRepository{tx: tx}
}

func (r *WalletRepository) Create(ctx context.Context, wallet *domain.Wallet) error {
	_, err := r.tx.Exec(ctx, `
		INSERT INTO wallets (id, player_id, currency, balance_minor, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`,
		wallet.ID(), wallet.PlayerID(), string(wallet.Currency()), wallet.Balance().AmountMinor(),
		wallet.Version(), wallet.CreatedAt(), wallet.UpdatedAt(),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == walletPlayerCurrencyUniqueConstraint {
			return repository.ErrAlreadyExists
		}
		return fmt.Errorf("create wallet: %w", err)
	}
	return nil
}

func (r *WalletRepository) Update(ctx context.Context, wallet *domain.Wallet) error {
	tag, err := r.tx.Exec(ctx, `
		UPDATE wallets SET balance_minor = $1, version = $2, updated_at = $3
		WHERE id = $4
	`, wallet.Balance().AmountMinor(), wallet.Version(), wallet.UpdatedAt(), wallet.ID())
	if err != nil {
		return fmt.Errorf("update wallet: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *WalletRepository) FindByIDForUpdate(ctx context.Context, id string) (*domain.Wallet, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT id, player_id, currency, balance_minor, version, created_at, updated_at
		FROM wallets WHERE id = $1 FOR UPDATE
	`, id)
	return scanWallet(row)
}

func (r *WalletRepository) FindByID(ctx context.Context, id string) (*domain.Wallet, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT id, player_id, currency, balance_minor, version, created_at, updated_at
		FROM wallets WHERE id = $1
	`, id)
	return scanWallet(row)
}

func scanWallet(row pgx.Row) (*domain.Wallet, error) {
	var (
		walletID, playerID, currency string
		balanceMinor, version        int64
		createdAt, updatedAt         time.Time
	)
	if err := row.Scan(&walletID, &playerID, &currency, &balanceMinor, &version, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan wallet: %w", err)
	}

	balance, err := domain.NewMoney(balanceMinor, domain.Currency(currency))
	if err != nil {
		return nil, err
	}
	return domain.RehydrateWallet(walletID, playerID, balance, version, createdAt, updatedAt)
}

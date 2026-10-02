package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type LedgerRepository struct {
	tx pgx.Tx
}

func NewLedgerRepository(tx pgx.Tx) *LedgerRepository {
	return &LedgerRepository{tx: tx}
}

func (r *LedgerRepository) Create(ctx context.Context, entry *domain.LedgerEntry) error {
	_, err := r.tx.Exec(ctx, `
		INSERT INTO wallet_ledger_entries (
			id, wallet_id, transaction_id, direction, amount_minor, currency,
			balance_before_minor, balance_after_minor, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		entry.ID(), entry.WalletID(), entry.TransactionID(), string(entry.Direction()),
		entry.Amount().AmountMinor(), string(entry.Amount().Currency()),
		entry.BalanceBefore().AmountMinor(), entry.BalanceAfter().AmountMinor(),
		entry.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("create ledger entry: %w", err)
	}
	return nil
}

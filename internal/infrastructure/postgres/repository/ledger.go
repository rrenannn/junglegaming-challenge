package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
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

func (r *LedgerRepository) SumByWallet(ctx context.Context, walletID string, currency domain.Currency) (domain.Money, error) {
	var sumMinor int64
	err := r.tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount_minor ELSE -amount_minor END), 0)
		FROM wallet_ledger_entries WHERE wallet_id = $1
	`, walletID).Scan(&sumMinor)
	if err != nil {
		return domain.Money{}, fmt.Errorf("sum ledger entries: %w", err)
	}
	return domain.NewMoney(sumMinor, currency)
}

func (r *LedgerRepository) ListByWallet(ctx context.Context, walletID string, after *repository.LedgerCursor, limit int) ([]*domain.LedgerEntry, error) {
	const columns = `id, wallet_id, transaction_id, direction, amount_minor, currency, balance_before_minor, balance_after_minor, created_at`

	var (
		rows pgx.Rows
		err  error
	)
	if after != nil {
		rows, err = r.tx.Query(ctx, `
			SELECT `+columns+`
			FROM wallet_ledger_entries
			WHERE wallet_id = $1 AND (created_at, id) > ($2, $3)
			ORDER BY created_at, id
			LIMIT $4
		`, walletID, after.CreatedAt, after.ID, limit)
	} else {
		rows, err = r.tx.Query(ctx, `
			SELECT `+columns+`
			FROM wallet_ledger_entries
			WHERE wallet_id = $1
			ORDER BY created_at, id
			LIMIT $2
		`, walletID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	defer rows.Close()

	var entries []*domain.LedgerEntry
	for rows.Next() {
		var (
			id, rowWalletID, transactionID, direction, currency string
			amountMinor, balanceBeforeMinor, balanceAfterMinor  int64
			createdAt                                           time.Time
		)
		if err := rows.Scan(&id, &rowWalletID, &transactionID, &direction, &amountMinor, &currency, &balanceBeforeMinor, &balanceAfterMinor, &createdAt); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}

		amount, err := domain.NewMoney(amountMinor, domain.Currency(currency))
		if err != nil {
			return nil, err
		}
		balanceBefore, err := domain.NewMoney(balanceBeforeMinor, domain.Currency(currency))
		if err != nil {
			return nil, err
		}
		balanceAfter, err := domain.NewMoney(balanceAfterMinor, domain.Currency(currency))
		if err != nil {
			return nil, err
		}

		entry, err := domain.RehydrateLedgerEntry(id, rowWalletID, transactionID, domain.MovementDirection(direction), amount, balanceBefore, balanceAfter, createdAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger entries: %w", err)
	}

	return entries, nil
}

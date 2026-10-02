package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type WagerTransactionRepository struct {
	tx pgx.Tx
}

func NewWagerTransactionRepository(tx pgx.Tx) *WagerTransactionRepository {
	return &WagerTransactionRepository{tx: tx}
}

func (r *WagerTransactionRepository) Create(ctx context.Context, wt *domain.WagerTransaction) error {
	var balanceAfterMinor sql.NullInt64
	if wt.BalanceAfter() != nil {
		balanceAfterMinor = sql.NullInt64{Int64: wt.BalanceAfter().AmountMinor(), Valid: true}
	}

	_, err := r.tx.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, wallet_id, provider_id, player_id, round_id, game_id,
			kind, status, direction, amount_minor, currency, balance_after_minor,
			reference_transaction_id, reversed_by_transaction_id, failure_code,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`,
		wt.ID(), wt.WalletID(), toNullString(wt.ProviderID()), wt.PlayerID(),
		toNullString(wt.RoundID()), toNullString(wt.GameID()),
		string(wt.Kind()), string(wt.Status()), string(wt.Direction()),
		wt.Amount().AmountMinor(), string(wt.Amount().Currency()), balanceAfterMinor,
		wt.ReferenceTransactionID(), wt.ReversedBy(), failureCodeToNullString(wt.FailureCode()),
		wt.CreatedAt(), wt.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("create wager transaction: %w", err)
	}
	return nil
}

func (r *WagerTransactionRepository) Update(ctx context.Context, wt *domain.WagerTransaction) error {
	var balanceAfterMinor sql.NullInt64
	if wt.BalanceAfter() != nil {
		balanceAfterMinor = sql.NullInt64{Int64: wt.BalanceAfter().AmountMinor(), Valid: true}
	}

	tag, err := r.tx.Exec(ctx, `
		UPDATE wager_transactions SET
			status = $1, direction = $2, balance_after_minor = $3,
			reference_transaction_id = $4, reversed_by_transaction_id = $5,
			failure_code = $6, updated_at = $7
		WHERE id = $8
	`,
		string(wt.Status()), string(wt.Direction()), balanceAfterMinor,
		wt.ReferenceTransactionID(), wt.ReversedBy(), failureCodeToNullString(wt.FailureCode()),
		wt.UpdatedAt(), wt.ID(),
	)
	if err != nil {
		return fmt.Errorf("update wager transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *WagerTransactionRepository) FindByIDForUpdate(ctx context.Context, id string) (*domain.WagerTransaction, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT id, wallet_id, provider_id, player_id, round_id, game_id,
			kind, status, direction, amount_minor, currency, balance_after_minor,
			reference_transaction_id, reversed_by_transaction_id, failure_code,
			created_at, updated_at
		FROM wager_transactions WHERE id = $1 FOR UPDATE
	`, id)

	var (
		txID, walletID, playerID, kind, status, direction, currency string
		providerID, roundID, gameID                                 sql.NullString
		amountMinor                                                 int64
		balanceAfterMinor                                           sql.NullInt64
		referenceID, reversedBy                                     *string
		failureCode                                                 sql.NullString
		createdAt, updatedAt                                        time.Time
	)
	err := row.Scan(
		&txID, &walletID, &providerID, &playerID, &roundID, &gameID,
		&kind, &status, &direction, &amountMinor, &currency, &balanceAfterMinor,
		&referenceID, &reversedBy, &failureCode,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("find wager transaction for update: %w", err)
	}

	amount, err := domain.NewMoney(amountMinor, domain.Currency(currency))
	if err != nil {
		return nil, err
	}

	var balanceAfter *domain.Money
	if balanceAfterMinor.Valid {
		m, err := domain.NewMoney(balanceAfterMinor.Int64, domain.Currency(currency))
		if err != nil {
			return nil, err
		}
		balanceAfter = &m
	}

	var failureCodePtr *domain.FailureCode
	if failureCode.Valid {
		code := domain.FailureCode(failureCode.String)
		failureCodePtr = &code
	}

	return domain.RehydrateWagerTransaction(
		txID, providerID.String, playerID, walletID, roundID.String, gameID.String,
		domain.TransactionKind(kind), domain.TransactionStatus(status), amount,
		domain.MovementDirection(direction), referenceID, reversedBy, failureCodePtr,
		balanceAfter, createdAt, updatedAt,
	)
}

func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func failureCodeToNullString(code *domain.FailureCode) sql.NullString {
	if code == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*code), Valid: true}
}

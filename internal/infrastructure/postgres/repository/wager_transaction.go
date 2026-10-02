package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

const (
	wagerTransactionsIdempotencyKeyConstraint = "wager_transactions_provider_idempotency_key_unique"
	wagerTransactionsExternalIDConstraint     = "wager_transactions_provider_external_id_unique"
)

const wagerTransactionColumns = `
	id, wallet_id, provider_id, player_id, round_id, game_id,
	kind, status, direction, amount_minor, currency, balance_after_minor,
	external_transaction_id, idempotency_key, payload_hash,
	reference_transaction_id, reference_external_transaction_id, reversed_by_transaction_id,
	failure_code, attempts, created_at, updated_at
`

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
			external_transaction_id, idempotency_key, payload_hash,
			reference_transaction_id, reference_external_transaction_id, reversed_by_transaction_id,
			failure_code, attempts, next_attempt_at, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,0,
			CASE WHEN $8 = 'PENDING_REFERENCE' THEN now() ELSE NULL END,
			$20,$21
		)
	`,
		wt.ID(), wt.WalletID(), toNullString(wt.ProviderID()), wt.PlayerID(),
		toNullString(wt.RoundID()), toNullString(wt.GameID()),
		string(wt.Kind()), string(wt.Status()), string(wt.Direction()),
		wt.Amount().AmountMinor(), string(wt.Amount().Currency()), balanceAfterMinor,
		toNullString(wt.ExternalTransactionID()), toNullString(wt.IdempotencyKey()), toNullString(wt.PayloadHash()),
		wt.ReferenceTransactionID(), wt.ReferenceExternalTransactionID(), wt.ReversedBy(),
		failureCodeToNullString(wt.FailureCode()),
		wt.CreatedAt(), wt.UpdatedAt(),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case wagerTransactionsIdempotencyKeyConstraint:
				return repository.ErrIdempotencyKeyConflict
			case wagerTransactionsExternalIDConstraint:
				return repository.ErrExternalTransactionConflict
			}
		}
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
	row := r.tx.QueryRow(ctx, `SELECT `+wagerTransactionColumns+` FROM wager_transactions WHERE id = $1 FOR UPDATE`, id)
	return scanWagerTransaction(row)
}

func (r *WagerTransactionRepository) FindByID(ctx context.Context, id string) (*domain.WagerTransaction, error) {
	row := r.tx.QueryRow(ctx, `SELECT `+wagerTransactionColumns+` FROM wager_transactions WHERE id = $1`, id)
	return scanWagerTransaction(row)
}

func (r *WagerTransactionRepository) FindByExternalIDForUpdate(ctx context.Context, providerID, externalTransactionID string) (*domain.WagerTransaction, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT `+wagerTransactionColumns+`
		FROM wager_transactions WHERE provider_id = $1 AND external_transaction_id = $2 FOR UPDATE
	`, providerID, externalTransactionID)
	return scanWagerTransaction(row)
}

func (r *WagerTransactionRepository) FindByIdempotencyKey(ctx context.Context, providerID, idempotencyKey string) (*domain.WagerTransaction, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT `+wagerTransactionColumns+`
		FROM wager_transactions WHERE provider_id = $1 AND idempotency_key = $2
	`, providerID, idempotencyKey)
	return scanWagerTransaction(row)
}

func (r *WagerTransactionRepository) FindDuePendingReferences(ctx context.Context, limit int) ([]*domain.WagerTransaction, error) {
	rows, err := r.tx.Query(ctx, `
		SELECT `+wagerTransactionColumns+`
		FROM wager_transactions
		WHERE status = 'PENDING_REFERENCE' AND next_attempt_at <= now()
		ORDER BY next_attempt_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("find due pending references: %w", err)
	}
	defer rows.Close()

	var results []*domain.WagerTransaction
	for rows.Next() {
		wt, err := scanWagerTransactionRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, wt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending references: %w", err)
	}
	return results, nil
}

func (r *WagerTransactionRepository) ReschedulePendingReference(ctx context.Context, id string, nextAttemptAt time.Time) error {
	tag, err := r.tx.Exec(ctx, `
		UPDATE wager_transactions SET attempts = attempts + 1, next_attempt_at = $1, updated_at = now()
		WHERE id = $2
	`, nextAttemptAt, id)
	if err != nil {
		return fmt.Errorf("reschedule pending reference: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

type wagerTransactionRow interface {
	Scan(dest ...any) error
}

func scanWagerTransaction(row pgx.Row) (*domain.WagerTransaction, error) {
	return scanWagerTransactionRow(row)
}

func scanWagerTransactionRow(row wagerTransactionRow) (*domain.WagerTransaction, error) {
	var (
		txID, walletID, playerID, kind, status, direction, currency string
		providerID, roundID, gameID                                 sql.NullString
		amountMinor                                                 int64
		balanceAfterMinor                                           sql.NullInt64
		externalTransactionID, idempotencyKey, payloadHash          sql.NullString
		referenceID, referenceExternalID, reversedBy                *string
		failureCode                                                 sql.NullString
		attempts                                                    int
		createdAt, updatedAt                                        time.Time
	)
	err := row.Scan(
		&txID, &walletID, &providerID, &playerID, &roundID, &gameID,
		&kind, &status, &direction, &amountMinor, &currency, &balanceAfterMinor,
		&externalTransactionID, &idempotencyKey, &payloadHash,
		&referenceID, &referenceExternalID, &reversedBy,
		&failureCode, &attempts, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("scan wager transaction: %w", err)
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

	return domain.RehydrateWagerTransaction(domain.RehydrateWagerTransactionParams{
		ID:                             txID,
		ProviderID:                     providerID.String,
		PlayerID:                       playerID,
		WalletID:                       walletID,
		RoundID:                        roundID.String,
		GameID:                         gameID.String,
		Kind:                           domain.TransactionKind(kind),
		Status:                         domain.TransactionStatus(status),
		Amount:                         amount,
		Direction:                      domain.MovementDirection(direction),
		ExternalTransactionID:          externalTransactionID.String,
		IdempotencyKey:                 idempotencyKey.String,
		PayloadHash:                    payloadHash.String,
		ReferenceTransactionID:         referenceID,
		ReferenceExternalTransactionID: referenceExternalID,
		ReversedBy:                     reversedBy,
		FailureCode:                    failureCodePtr,
		BalanceAfter:                   balanceAfter,
		Attempts:                       attempts,
		CreatedAt:                      createdAt,
		UpdatedAt:                      updatedAt,
	})
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

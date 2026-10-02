package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests bypass the application entirely — raw SQL against the pool,
// simulating a bug or a careless hand reaching directly into the database.
// They prove the schema itself enforces the rules, not just the Go code:
// the repository doesn't even expose Update/Delete for the ledger.

// seedProcessedBetLedgerEntry processes a real 30.00 BET through the
// service (so every referenced row is valid) and returns the resulting
// ledger entry id and wager transaction id to tamper with directly.
func seedProcessedBetLedgerEntry(t *testing.T, svc *ProcessWagerService, pool *pgxpool.Pool, walletID, playerID string) (ledgerID, transactionID string) {
	t.Helper()

	result, err := svc.Execute(context.Background(), betCommand(walletID, playerID, "30.00"))
	if err != nil {
		t.Fatalf("seed bet: %v", err)
	}

	err = pool.QueryRow(context.Background(), `SELECT id FROM wallet_ledger_entries WHERE transaction_id = $1`, result.TransactionID).Scan(&ledgerID)
	if err != nil {
		t.Fatalf("find seeded ledger entry: %v", err)
	}
	return ledgerID, result.TransactionID
}

func TestSchemaConstraint_LedgerEntriesRejectUpdate(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-schema-1", "100.00")
	ledgerID, _ := seedProcessedBetLedgerEntry(t, svc, pool, walletID, "player-schema-1")

	_, err := pool.Exec(context.Background(), `UPDATE wallet_ledger_entries SET amount_minor = 999 WHERE id = $1`, ledgerID)
	if err == nil {
		t.Fatal("expected an error updating a ledger row, the table must be append-only")
	}
}

func TestSchemaConstraint_LedgerEntriesRejectDelete(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-schema-2", "100.00")
	ledgerID, _ := seedProcessedBetLedgerEntry(t, svc, pool, walletID, "player-schema-2")

	_, err := pool.Exec(context.Background(), `DELETE FROM wallet_ledger_entries WHERE id = $1`, ledgerID)
	if err == nil {
		t.Fatal("expected an error deleting a ledger row, the table must be append-only")
	}
}

func TestSchemaConstraint_LedgerEquationIsEnforced(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-schema-3", "100.00")
	_, txID := seedProcessedBetLedgerEntry(t, svc, pool, walletID, "player-schema-3")

	// balance_after_minor (9000) doesn't match balance_before - amount (10000 - 3000 = 7000).
	_, err := pool.Exec(context.Background(), `
		INSERT INTO wallet_ledger_entries (
			id, wallet_id, transaction_id, direction, amount_minor, currency,
			balance_before_minor, balance_after_minor, created_at
		) VALUES (gen_random_uuid(), $1, $2, 'DEBIT', 3000, 'BRL', 10000, 9000, now())
	`, walletID, txID)
	if err == nil {
		t.Fatal("expected an error inserting a ledger row whose equation doesn't balance")
	}
}

func TestSchemaConstraint_WalletBalanceCannotGoNegative(t *testing.T) {
	pool := newTestPool(t)
	walletID := seedWallet(t, pool, "player-schema-4", "100.00")

	_, err := pool.Exec(context.Background(), `UPDATE wallets SET balance_minor = -1 WHERE id = $1`, walletID)
	if err == nil {
		t.Fatal("expected an error setting a negative wallet balance")
	}
}

func TestSchemaConstraint_WagerTransactionAmountCannotGoNegative(t *testing.T) {
	svc, pool := newTestService(t)
	walletID := seedWallet(t, pool, "player-schema-5", "100.00")
	_, txID := seedProcessedBetLedgerEntry(t, svc, pool, walletID, "player-schema-5")

	_, err := pool.Exec(context.Background(), `UPDATE wager_transactions SET amount_minor = -1 WHERE id = $1`, txID)
	if err == nil {
		t.Fatal("expected an error setting a negative wager transaction amount")
	}
}

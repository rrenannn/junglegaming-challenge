package domain

import (
	"testing"
	"time"
)

func money(t *testing.T, decimal string) Money {
	t.Helper()
	m, err := ParseMoney(decimal, BRL)
	if err != nil {
		t.Fatalf("ParseMoney(%q): %v", decimal, err)
	}
	return m
}

func newWalletWithBalance(t *testing.T, decimal string) *Wallet {
	t.Helper()
	wallet, err := NewWallet("wallet-1", "player-1", money(t, decimal), time.Now())
	if err != nil {
		t.Fatalf("NewWallet: %v", err)
	}
	return wallet
}

func newExternalTx(t *testing.T, id string, kind TransactionKind, amount string) *WagerTransaction {
	t.Helper()
	tx, err := NewWagerTransaction(id, "provider-a", "player-1", "wallet-1", "round-1", "game-1", kind, money(t, amount), time.Now())
	if err != nil {
		t.Fatalf("NewWagerTransaction(%s): %v", kind, err)
	}
	return tx
}

func TestNewWagerTransaction_OpeningRejectsExternalMetadata(t *testing.T) {
	zero, _ := ZeroMoney(BRL)
	_, err := NewWagerTransaction("tx-1", "provider-a", "player-1", "wallet-1", "", "", KindOpening, zero, time.Now())
	if !HasFailureCode(err, FailureInvalidTransactionKind) {
		t.Fatalf("got %v, want INVALID_TRANSACTION_KIND", err)
	}
}

func TestNewWagerTransaction_ExternalRequiresMetadata(t *testing.T) {
	amount := money(t, "10.00")
	_, err := NewWagerTransaction("tx-1", "", "player-1", "wallet-1", "round-1", "game-1", KindBet, amount, time.Now())
	if !HasFailureCode(err, FailureInvalidTransactionKind) {
		t.Fatalf("got %v, want INVALID_TRANSACTION_KIND", err)
	}
}

func TestNewWagerTransaction_KindAmountRules(t *testing.T) {
	zero, _ := ZeroMoney(BRL)
	positive := money(t, "10.00")

	cases := []struct {
		kind    TransactionKind
		amount  Money
		wantErr bool
	}{
		{KindBet, positive, false},
		{KindBet, zero, true},
		{KindWin, positive, false},
		{KindWin, zero, true},
		{KindLoss, zero, false},
		{KindLoss, positive, true},
		{KindRefund, positive, false},
		{KindRefund, zero, true},
		{KindRollback, positive, false},
		{KindRollback, zero, true},
	}

	for _, tc := range cases {
		_, err := NewWagerTransaction("tx-1", "provider-a", "player-1", "wallet-1", "round-1", "game-1", tc.kind, tc.amount, time.Now())
		if tc.wantErr && err == nil {
			t.Errorf("%s with amount %s: expected error, got none", tc.kind, tc.amount.Decimal())
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s with amount %s: unexpected error %v", tc.kind, tc.amount.Decimal(), err)
		}
	}
}

func TestProcessOpening(t *testing.T) {
	now := time.Now()
	balance := money(t, "100.00")
	wallet, err := NewWallet("wallet-1", "player-1", balance, now)
	if err != nil {
		t.Fatalf("NewWallet: %v", err)
	}

	tx, err := NewWagerTransaction("tx-1", "", "player-1", "wallet-1", "", "", KindOpening, balance, now)
	if err != nil {
		t.Fatalf("NewWagerTransaction: %v", err)
	}

	if err := tx.ProcessOpening(wallet, now); err != nil {
		t.Fatalf("ProcessOpening: %v", err)
	}
	if tx.Status() != StatusProcessed {
		t.Fatalf("Status = %s, want PROCESSED", tx.Status())
	}
	if tx.Direction() != DirectionCredit {
		t.Fatalf("Direction = %s, want CREDIT", tx.Direction())
	}
	if tx.BalanceAfter() == nil || tx.BalanceAfter().Decimal() != "100.00" {
		t.Fatalf("BalanceAfter = %v, want 100.00", tx.BalanceAfter())
	}
}

func TestProcessBet_DebitsWallet(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	tx := newExternalTx(t, "tx-1", KindBet, "80.00")

	if err := tx.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}
	if wallet.Balance().Decimal() != "20.00" {
		t.Fatalf("wallet balance = %s, want 20.00", wallet.Balance().Decimal())
	}
	if tx.Direction() != DirectionDebit || tx.Status() != StatusProcessed {
		t.Fatalf("tx = %+v, want processed debit", tx)
	}
}

func TestProcessBet_InsufficientFunds(t *testing.T) {
	wallet := newWalletWithBalance(t, "50.00")
	tx := newExternalTx(t, "tx-1", KindBet, "50.01")

	err := tx.ProcessBet(wallet, time.Now())
	if !HasFailureCode(err, FailureInsufficientFunds) {
		t.Fatalf("got %v, want INSUFFICIENT_FUNDS", err)
	}
	if tx.Status() == StatusProcessed {
		t.Fatal("transaction must not be processed on failure")
	}
}

func TestProcessWin_CreditsWallet(t *testing.T) {
	wallet := newWalletWithBalance(t, "20.00")
	tx := newExternalTx(t, "tx-1", KindWin, "30.00")

	if err := tx.ProcessWin(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessWin: %v", err)
	}
	if wallet.Balance().Decimal() != "50.00" {
		t.Fatalf("wallet balance = %s, want 50.00", wallet.Balance().Decimal())
	}
	if tx.Direction() != DirectionCredit {
		t.Fatalf("Direction = %s, want CREDIT", tx.Direction())
	}
}

func TestProcessLoss_NoMovement(t *testing.T) {
	wallet := newWalletWithBalance(t, "20.00")
	tx := newExternalTx(t, "tx-1", KindLoss, "0.00")

	if err := tx.ProcessLoss(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessLoss: %v", err)
	}
	if wallet.Balance().Decimal() != "20.00" {
		t.Fatalf("wallet balance = %s, want unchanged 20.00", wallet.Balance().Decimal())
	}
	if tx.Direction() != DirectionNone {
		t.Fatalf("Direction = %s, want NONE", tx.Direction())
	}
}

func TestProcessRefund_RefundsProcessedBet(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet := newExternalTx(t, "tx-bet", KindBet, "80.00")
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}

	refund := newExternalTx(t, "tx-refund", KindRefund, "80.00")
	if err := refund.ProcessRefund(wallet, bet, time.Now()); err != nil {
		t.Fatalf("ProcessRefund: %v", err)
	}
	if wallet.Balance().Decimal() != "100.00" {
		t.Fatalf("wallet balance = %s, want 100.00", wallet.Balance().Decimal())
	}
	if bet.ReversedBy() == nil || *bet.ReversedBy() != "tx-refund" {
		t.Fatalf("bet.ReversedBy() = %v, want tx-refund", bet.ReversedBy())
	}
	if refund.ReferenceTransactionID() == nil || *refund.ReferenceTransactionID() != "tx-bet" {
		t.Fatalf("refund.ReferenceTransactionID() = %v, want tx-bet", refund.ReferenceTransactionID())
	}
}

func TestProcessRefund_RejectsNonBetReference(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	win := newExternalTx(t, "tx-win", KindWin, "30.00")
	if err := win.ProcessWin(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessWin: %v", err)
	}

	refund := newExternalTx(t, "tx-refund", KindRefund, "30.00")
	err := refund.ProcessRefund(wallet, win, time.Now())
	if !HasFailureCode(err, FailureReferenceMismatch) {
		t.Fatalf("got %v, want REFERENCE_MISMATCH", err)
	}
}

func TestProcessRefund_RejectsUnprocessedReference(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet := newExternalTx(t, "tx-bet", KindBet, "80.00")

	refund := newExternalTx(t, "tx-refund", KindRefund, "80.00")
	err := refund.ProcessRefund(wallet, bet, time.Now())
	if !HasFailureCode(err, FailureReferenceNotProcessed) {
		t.Fatalf("got %v, want REFERENCE_NOT_PROCESSED", err)
	}
}

func TestProcessRefund_RejectsAmountMismatch(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet := newExternalTx(t, "tx-bet", KindBet, "80.00")
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}

	refund := newExternalTx(t, "tx-refund", KindRefund, "50.00")
	err := refund.ProcessRefund(wallet, bet, time.Now())
	if !HasFailureCode(err, FailureReferenceMismatch) {
		t.Fatalf("got %v, want REFERENCE_MISMATCH", err)
	}
}

func TestProcessRefund_RejectsSecondDirectReversal(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet := newExternalTx(t, "tx-bet", KindBet, "80.00")
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}

	firstRefund := newExternalTx(t, "tx-refund-1", KindRefund, "80.00")
	if err := firstRefund.ProcessRefund(wallet, bet, time.Now()); err != nil {
		t.Fatalf("first refund: %v", err)
	}

	secondRefund := newExternalTx(t, "tx-refund-2", KindRefund, "80.00")
	err := secondRefund.ProcessRefund(wallet, bet, time.Now())
	if !HasFailureCode(err, FailureAlreadyReversed) {
		t.Fatalf("got %v, want ALREADY_REVERSED", err)
	}
}

func TestProcessRollback_UndoesBet(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet := newExternalTx(t, "tx-bet", KindBet, "80.00")
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}
	if wallet.Balance().Decimal() != "20.00" {
		t.Fatalf("precondition balance = %s, want 20.00", wallet.Balance().Decimal())
	}

	rollback := newExternalTx(t, "tx-rollback", KindRollback, "80.00")
	if err := rollback.ProcessRollback(wallet, bet, time.Now()); err != nil {
		t.Fatalf("ProcessRollback: %v", err)
	}
	if wallet.Balance().Decimal() != "100.00" {
		t.Fatalf("wallet balance = %s, want 100.00", wallet.Balance().Decimal())
	}
	if rollback.Direction() != DirectionCredit {
		t.Fatalf("Direction = %s, want CREDIT (inverse of debit)", rollback.Direction())
	}
}

func TestProcessRollback_UndoesWin(t *testing.T) {
	wallet := newWalletWithBalance(t, "20.00")
	win := newExternalTx(t, "tx-win", KindWin, "30.00")
	if err := win.ProcessWin(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessWin: %v", err)
	}

	rollback := newExternalTx(t, "tx-rollback", KindRollback, "30.00")
	if err := rollback.ProcessRollback(wallet, win, time.Now()); err != nil {
		t.Fatalf("ProcessRollback: %v", err)
	}
	if wallet.Balance().Decimal() != "20.00" {
		t.Fatalf("wallet balance = %s, want 20.00", wallet.Balance().Decimal())
	}
	if rollback.Direction() != DirectionDebit {
		t.Fatalf("Direction = %s, want DEBIT (inverse of credit)", rollback.Direction())
	}
}

func TestProcessRollback_UndoesRefundByReferencingRefundItself(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet := newExternalTx(t, "tx-bet", KindBet, "80.00")
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}
	refund := newExternalTx(t, "tx-refund", KindRefund, "80.00")
	if err := refund.ProcessRefund(wallet, bet, time.Now()); err != nil {
		t.Fatalf("ProcessRefund: %v", err)
	}
	// wallet is back to 100.00; bet is marked reversed by the refund.

	rollback := newExternalTx(t, "tx-rollback", KindRollback, "80.00")
	if err := rollback.ProcessRollback(wallet, refund, time.Now()); err != nil {
		t.Fatalf("ProcessRollback against refund: %v", err)
	}
	if wallet.Balance().Decimal() != "20.00" {
		t.Fatalf("wallet balance = %s, want 20.00", wallet.Balance().Decimal())
	}
	if refund.ReversedBy() == nil || *refund.ReversedBy() != "tx-rollback" {
		t.Fatalf("refund.ReversedBy() = %v, want tx-rollback", refund.ReversedBy())
	}
	// The original bet itself was never directly reversed by the rollback.
	if bet.ReversedBy() == nil || *bet.ReversedBy() != "tx-refund" {
		t.Fatalf("bet.ReversedBy() must still point at the refund, got %v", bet.ReversedBy())
	}
}

func TestProcessRollback_InsufficientFundsMapsToReversalCode(t *testing.T) {
	wallet := newWalletWithBalance(t, "20.00")
	win := newExternalTx(t, "tx-win", KindWin, "30.00")
	if err := win.ProcessWin(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessWin: %v", err)
	}
	// Spend the winnings so the wallet can no longer afford the rollback debit.
	bet := newExternalTx(t, "tx-bet", KindBet, "45.00")
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}
	if wallet.Balance().Decimal() != "5.00" {
		t.Fatalf("precondition balance = %s, want 5.00", wallet.Balance().Decimal())
	}

	rollback := newExternalTx(t, "tx-rollback", KindRollback, "30.00")
	err := rollback.ProcessRollback(wallet, win, time.Now())
	if !HasFailureCode(err, FailureInsufficientFundsForReversal) {
		t.Fatalf("got %v, want INSUFFICIENT_FUNDS_FOR_REVERSAL", err)
	}
}

func TestProcessRollback_RejectsFieldMismatch(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet, err := NewWagerTransaction("tx-bet", "provider-a", "player-1", "wallet-1", "round-1", "game-1", KindBet, money(t, "80.00"), time.Now())
	if err != nil {
		t.Fatalf("NewWagerTransaction: %v", err)
	}
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}

	rollback, err := NewWagerTransaction("tx-rollback", "provider-b", "player-1", "wallet-1", "round-1", "game-1", KindRollback, money(t, "80.00"), time.Now())
	if err != nil {
		t.Fatalf("NewWagerTransaction: %v", err)
	}
	err = rollback.ProcessRollback(wallet, bet, time.Now())
	if !HasFailureCode(err, FailureReferenceMismatch) {
		t.Fatalf("got %v, want REFERENCE_MISMATCH", err)
	}
}

func TestMarkRejected_And_MarkFailed(t *testing.T) {
	tx := newExternalTx(t, "tx-1", KindBet, "10.00")

	if err := tx.MarkRejected(FailureInsufficientFunds, time.Now()); err != nil {
		t.Fatalf("MarkRejected: %v", err)
	}
	if tx.Status() != StatusRejected {
		t.Fatalf("Status = %s, want REJECTED", tx.Status())
	}
	if tx.FailureCode() == nil || *tx.FailureCode() != FailureInsufficientFunds {
		t.Fatalf("FailureCode = %v, want INSUFFICIENT_FUNDS", tx.FailureCode())
	}

	if err := tx.MarkFailed(time.Now()); !HasFailureCode(err, FailureInvalidState) {
		t.Fatalf("got %v, want INVALID_STATE (already terminal)", err)
	}
}

func TestMarkPendingReference_OnlyForReversals(t *testing.T) {
	bet := newExternalTx(t, "tx-1", KindBet, "10.00")
	if err := bet.MarkPendingReference(time.Now()); !HasFailureCode(err, FailureInvalidTransactionKind) {
		t.Fatalf("got %v, want INVALID_TRANSACTION_KIND", err)
	}

	refund := newExternalTx(t, "tx-2", KindRefund, "10.00")
	if err := refund.MarkPendingReference(time.Now()); err != nil {
		t.Fatalf("MarkPendingReference: %v", err)
	}
	if refund.Status() != StatusPendingReference {
		t.Fatalf("Status = %s, want PENDING_REFERENCE", refund.Status())
	}
}

func TestTerminalStatus_RejectsFurtherTransitions(t *testing.T) {
	wallet := newWalletWithBalance(t, "100.00")
	bet := newExternalTx(t, "tx-1", KindBet, "10.00")
	if err := bet.ProcessBet(wallet, time.Now()); err != nil {
		t.Fatalf("ProcessBet: %v", err)
	}

	if err := bet.ProcessBet(wallet, time.Now()); !HasFailureCode(err, FailureInvalidState) {
		t.Fatalf("got %v, want INVALID_STATE", err)
	}
	if err := bet.MarkRejected(FailureInvalidAmount, time.Now()); !HasFailureCode(err, FailureInvalidState) {
		t.Fatalf("got %v, want INVALID_STATE", err)
	}
}

func TestRehydrateWagerTransaction_NoValidation(t *testing.T) {
	tx, err := RehydrateWagerTransaction(
		"tx-1", "provider-a", "player-1", "wallet-1", "round-1", "game-1",
		KindBet, StatusProcessed, money(t, "10.00"), DirectionDebit,
		nil, nil, nil, nil,
		time.Now(), time.Now(),
	)
	if err != nil {
		t.Fatalf("RehydrateWagerTransaction: %v", err)
	}
	if tx.Status() != StatusProcessed {
		t.Fatalf("Status = %s, want PROCESSED", tx.Status())
	}
}

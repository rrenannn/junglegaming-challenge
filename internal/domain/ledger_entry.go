package domain

import (
	"strings"
	"time"
)

type LedgerEntry struct {
	id            string
	walletID      string
	transactionID string
	direction     MovementDirection
	amount        Money
	balanceBefore Money
	balanceAfter  Money
	createdAt     time.Time
}

func NewLedgerEntry(id, walletID, transactionID string, direction MovementDirection, amount, balanceBefore, balanceAfter Money, now time.Time) (*LedgerEntry, error) {
	if strings.TrimSpace(id) == "" {
		return nil, NewError(FailureInvalidState, "ledger entry id is required")
	}
	if strings.TrimSpace(walletID) == "" {
		return nil, NewError(FailureInvalidState, "wallet id is required")
	}
	if strings.TrimSpace(transactionID) == "" {
		return nil, NewError(FailureInvalidState, "transaction id is required")
	}
	if direction != DirectionDebit && direction != DirectionCredit {
		return nil, NewError(FailureInvalidState, "ledger entry direction must be debit or credit")
	}
	if !amount.IsPositive() {
		return nil, NewError(FailureInvalidAmount, "ledger entry amount must be positive")
	}
	if balanceAfter.IsNegative() {
		return nil, NewError(FailureInsufficientFunds, "ledger entry balance after must not be negative")
	}

	var (
		expected Money
		err      error
	)
	switch direction {
	case DirectionDebit:
		expected, err = balanceBefore.Sub(amount)
	case DirectionCredit:
		expected, err = balanceBefore.Add(amount)
	}
	if err != nil {
		return nil, err
	}
	cmp, err := expected.Compare(balanceAfter)
	if err != nil {
		return nil, err
	}
	if cmp != 0 {
		return nil, NewError(FailureInvalidState, "balance after does not match balance before and amount")
	}

	return &LedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     now,
	}, nil
}

func RehydrateLedgerEntry(id, walletID, transactionID string, direction MovementDirection, amount, balanceBefore, balanceAfter Money, createdAt time.Time) (*LedgerEntry, error) {
	if strings.TrimSpace(id) == "" {
		return nil, NewError(FailureInvalidState, "ledger entry id is required")
	}
	return &LedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt,
	}, nil
}

func (e *LedgerEntry) ID() string                   { return e.id }
func (e *LedgerEntry) WalletID() string             { return e.walletID }
func (e *LedgerEntry) TransactionID() string        { return e.transactionID }
func (e *LedgerEntry) Direction() MovementDirection { return e.direction }
func (e *LedgerEntry) Amount() Money                { return e.amount }
func (e *LedgerEntry) BalanceBefore() Money         { return e.balanceBefore }
func (e *LedgerEntry) BalanceAfter() Money          { return e.balanceAfter }
func (e *LedgerEntry) CreatedAt() time.Time         { return e.createdAt }

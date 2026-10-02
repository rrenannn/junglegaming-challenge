package domain

import (
	"strings"
	"time"
)

type Wallet struct {
	id        string
	playerID  string
	currency  Currency
	balance   Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// A positive openingBalance is the caller's responsibility to also record
// as an OPENING transaction and ledger entry.
func NewWallet(id, playerID string, openingBalance Money, now time.Time) (*Wallet, error) {
	if strings.TrimSpace(id) == "" {
		return nil, NewError(FailureInvalidState, "wallet id is required")
	}
	if strings.TrimSpace(playerID) == "" {
		return nil, NewError(FailureInvalidState, "player id is required")
	}
	if openingBalance.IsNegative() {
		return nil, NewError(FailureInvalidAmount, "opening balance must not be negative")
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  openingBalance.Currency(),
		balance:   openingBalance,
		version:   1,
		createdAt: now,
		updatedAt: now,
	}, nil
}

func RehydrateWallet(id, playerID string, balance Money, version int64, createdAt, updatedAt time.Time) (*Wallet, error) {
	if strings.TrimSpace(id) == "" {
		return nil, NewError(FailureInvalidState, "wallet id is required")
	}
	if strings.TrimSpace(playerID) == "" {
		return nil, NewError(FailureInvalidState, "player id is required")
	}
	if version < 1 {
		return nil, NewError(FailureInvalidState, "wallet version must be at least 1")
	}
	if balance.IsNegative() {
		return nil, NewError(FailureInvalidAmount, "wallet balance must not be negative")
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  balance.Currency(),
		balance:   balance,
		version:   version,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

func (w *Wallet) ID() string           { return w.id }
func (w *Wallet) PlayerID() string     { return w.playerID }
func (w *Wallet) Currency() Currency   { return w.currency }
func (w *Wallet) Balance() Money       { return w.balance }
func (w *Wallet) Version() int64       { return w.version }
func (w *Wallet) CreatedAt() time.Time { return w.createdAt }
func (w *Wallet) UpdatedAt() time.Time { return w.updatedAt }

func (w *Wallet) Debit(amount Money, now time.Time) error {
	if amount.Currency() != w.currency {
		return NewError(FailureCurrencyMismatch, "debit currency does not match wallet currency")
	}
	if !amount.IsPositive() {
		return NewError(FailureInvalidAmount, "debit amount must be positive")
	}

	newBalance, err := w.balance.Sub(amount)
	if err != nil {
		return err
	}
	if newBalance.IsNegative() {
		return NewError(FailureInsufficientFunds, "insufficient funds for debit")
	}

	w.balance = newBalance
	w.version++
	w.updatedAt = now
	return nil
}

func (w *Wallet) Credit(amount Money, now time.Time) error {
	if amount.Currency() != w.currency {
		return NewError(FailureCurrencyMismatch, "credit currency does not match wallet currency")
	}
	if !amount.IsPositive() {
		return NewError(FailureInvalidAmount, "credit amount must be positive")
	}

	newBalance, err := w.balance.Add(amount)
	if err != nil {
		return err
	}

	w.balance = newBalance
	w.version++
	w.updatedAt = now
	return nil
}

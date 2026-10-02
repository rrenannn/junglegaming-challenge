package domain

import (
	"testing"
	"time"
)

func TestNewWallet_ZeroOpeningBalance(t *testing.T) {
	zero, _ := ZeroMoney(BRL)
	now := time.Now()

	wallet, err := NewWallet("wallet-1", "player-1", zero, now)
	if err != nil {
		t.Fatalf("NewWallet: %v", err)
	}
	if wallet.Version() != 1 {
		t.Fatalf("Version = %d, want 1", wallet.Version())
	}
	if !wallet.Balance().IsZero() {
		t.Fatalf("Balance = %v, want zero", wallet.Balance())
	}
}

func TestNewWallet_NegativeOpeningBalanceRejected(t *testing.T) {
	negative := Money{amountMinor: -100, currency: BRL}
	if _, err := NewWallet("wallet-1", "player-1", negative, time.Now()); err == nil {
		t.Fatal("expected error for negative opening balance")
	}
}

func TestNewWallet_RequiresIdentifiers(t *testing.T) {
	zero, _ := ZeroMoney(BRL)
	if _, err := NewWallet("", "player-1", zero, time.Now()); err == nil {
		t.Fatal("expected error for missing wallet id")
	}
	if _, err := NewWallet("wallet-1", "", zero, time.Now()); err == nil {
		t.Fatal("expected error for missing player id")
	}
}

func TestWallet_DebitAndCredit(t *testing.T) {
	opening, _ := ParseMoney("100.00", BRL)
	now := time.Now()
	wallet, err := NewWallet("wallet-1", "player-1", opening, now)
	if err != nil {
		t.Fatalf("NewWallet: %v", err)
	}

	bet, _ := ParseMoney("80.00", BRL)
	later := now.Add(time.Minute)
	if err := wallet.Debit(bet, later); err != nil {
		t.Fatalf("Debit: %v", err)
	}
	if wallet.Balance().Decimal() != "20.00" {
		t.Fatalf("Balance = %s, want 20.00", wallet.Balance().Decimal())
	}
	if wallet.Version() != 2 {
		t.Fatalf("Version = %d, want 2", wallet.Version())
	}
	if !wallet.UpdatedAt().Equal(later) {
		t.Fatalf("UpdatedAt = %v, want %v", wallet.UpdatedAt(), later)
	}

	win, _ := ParseMoney("50.00", BRL)
	if err := wallet.Credit(win, later); err != nil {
		t.Fatalf("Credit: %v", err)
	}
	if wallet.Balance().Decimal() != "70.00" {
		t.Fatalf("Balance = %s, want 70.00", wallet.Balance().Decimal())
	}
	if wallet.Version() != 3 {
		t.Fatalf("Version = %d, want 3", wallet.Version())
	}
}

func TestWallet_DebitInsufficientFunds(t *testing.T) {
	opening, _ := ParseMoney("50.00", BRL)
	wallet, _ := NewWallet("wallet-1", "player-1", opening, time.Now())

	tooMuch, _ := ParseMoney("50.01", BRL)
	err := wallet.Debit(tooMuch, time.Now())
	if !HasFailureCode(err, FailureInsufficientFunds) {
		t.Fatalf("Debit error = %v, want INSUFFICIENT_FUNDS", err)
	}
	if wallet.Version() != 1 {
		t.Fatalf("Version should not change on failed debit, got %d", wallet.Version())
	}
}

func TestWallet_TwoConcurrentBetsAgainstSameBalance(t *testing.T) {
	// Mirrors the day-1 milestone scenario: two 80.00 bets against a
	// 100.00 balance — only one may succeed, leaving exactly 20.00.
	opening, _ := ParseMoney("100.00", BRL)
	wallet, _ := NewWallet("wallet-1", "player-1", opening, time.Now())
	bet, _ := ParseMoney("80.00", BRL)

	firstErr := wallet.Debit(bet, time.Now())
	secondErr := wallet.Debit(bet, time.Now())

	if firstErr != nil {
		t.Fatalf("first debit should succeed, got %v", firstErr)
	}
	if !HasFailureCode(secondErr, FailureInsufficientFunds) {
		t.Fatalf("second debit = %v, want INSUFFICIENT_FUNDS", secondErr)
	}
	if wallet.Balance().Decimal() != "20.00" {
		t.Fatalf("Balance = %s, want 20.00", wallet.Balance().Decimal())
	}
}

func TestWallet_DebitCurrencyMismatch(t *testing.T) {
	opening, _ := ParseMoney("50.00", BRL)
	wallet, _ := NewWallet("wallet-1", "player-1", opening, time.Now())

	foreign := Money{amountMinor: 100, currency: "USD"}
	err := wallet.Debit(foreign, time.Now())
	if !HasFailureCode(err, FailureCurrencyMismatch) {
		t.Fatalf("Debit error = %v, want CURRENCY_MISMATCH", err)
	}
}

func TestWallet_DebitNonPositiveAmountRejected(t *testing.T) {
	opening, _ := ParseMoney("50.00", BRL)
	wallet, _ := NewWallet("wallet-1", "player-1", opening, time.Now())

	zero, _ := ZeroMoney(BRL)
	if err := wallet.Debit(zero, time.Now()); !HasFailureCode(err, FailureInvalidAmount) {
		t.Fatalf("Debit(zero) error = %v, want INVALID_AMOUNT", err)
	}
}

func TestRehydrateWallet_NoTransitions(t *testing.T) {
	balance, _ := ParseMoney("42.00", BRL)
	created := time.Now().Add(-time.Hour)
	updated := time.Now()

	wallet, err := RehydrateWallet("wallet-1", "player-1", balance, 7, created, updated)
	if err != nil {
		t.Fatalf("RehydrateWallet: %v", err)
	}
	if wallet.Version() != 7 {
		t.Fatalf("Version = %d, want 7", wallet.Version())
	}
	if wallet.Balance().Decimal() != "42.00" {
		t.Fatalf("Balance = %s, want 42.00", wallet.Balance().Decimal())
	}
	if !wallet.CreatedAt().Equal(created) || !wallet.UpdatedAt().Equal(updated) {
		t.Fatal("rehydration must preserve original timestamps")
	}
}

func TestRehydrateWallet_RejectsInvalidVersion(t *testing.T) {
	balance, _ := ParseMoney("10.00", BRL)
	if _, err := RehydrateWallet("wallet-1", "player-1", balance, 0, time.Now(), time.Now()); err == nil {
		t.Fatal("expected error for version below 1")
	}
}

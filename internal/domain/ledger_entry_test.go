package domain

import (
	"testing"
	"time"
)

func TestNewLedgerEntry_Debit(t *testing.T) {
	before := money(t, "100.00")
	after := money(t, "20.00")
	amount := money(t, "80.00")

	entry, err := NewLedgerEntry("entry-1", "wallet-1", "tx-1", DirectionDebit, amount, before, after, time.Now())
	if err != nil {
		t.Fatalf("NewLedgerEntry: %v", err)
	}
	if entry.Direction() != DirectionDebit {
		t.Fatalf("Direction = %s, want DEBIT", entry.Direction())
	}
}

func TestNewLedgerEntry_Credit(t *testing.T) {
	before := money(t, "20.00")
	after := money(t, "50.00")
	amount := money(t, "30.00")

	if _, err := NewLedgerEntry("entry-1", "wallet-1", "tx-1", DirectionCredit, amount, before, after, time.Now()); err != nil {
		t.Fatalf("NewLedgerEntry: %v", err)
	}
}

func TestNewLedgerEntry_RejectsBrokenEquation(t *testing.T) {
	before := money(t, "100.00")
	after := money(t, "25.00") // should have been 20.00
	amount := money(t, "80.00")

	_, err := NewLedgerEntry("entry-1", "wallet-1", "tx-1", DirectionDebit, amount, before, after, time.Now())
	if !HasFailureCode(err, FailureInvalidState) {
		t.Fatalf("got %v, want INVALID_STATE", err)
	}
}

func TestNewLedgerEntry_RejectsNegativeBalanceAfter(t *testing.T) {
	before := money(t, "10.00")
	amount := money(t, "20.00")
	after := Money{amountMinor: -1000, currency: BRL}

	_, err := NewLedgerEntry("entry-1", "wallet-1", "tx-1", DirectionDebit, amount, before, after, time.Now())
	if !HasFailureCode(err, FailureInsufficientFunds) {
		t.Fatalf("got %v, want INSUFFICIENT_FUNDS", err)
	}
}

func TestNewLedgerEntry_RejectsNonPositiveAmount(t *testing.T) {
	zero, _ := ZeroMoney(BRL)
	before := money(t, "10.00")

	_, err := NewLedgerEntry("entry-1", "wallet-1", "tx-1", DirectionDebit, zero, before, before, time.Now())
	if !HasFailureCode(err, FailureInvalidAmount) {
		t.Fatalf("got %v, want INVALID_AMOUNT", err)
	}
}

func TestNewLedgerEntry_RejectsNoneDirection(t *testing.T) {
	amount := money(t, "10.00")
	before := money(t, "10.00")
	after := money(t, "20.00")

	_, err := NewLedgerEntry("entry-1", "wallet-1", "tx-1", DirectionNone, amount, before, after, time.Now())
	if !HasFailureCode(err, FailureInvalidState) {
		t.Fatalf("got %v, want INVALID_STATE", err)
	}
}

func TestRehydrateLedgerEntry_SkipsValidation(t *testing.T) {
	// Even a mathematically broken entry can be rehydrated: persisted rows
	// are trusted, not re-derived.
	amount := money(t, "10.00")
	before := money(t, "10.00")
	after := money(t, "999.00")

	entry, err := RehydrateLedgerEntry("entry-1", "wallet-1", "tx-1", DirectionDebit, amount, before, after, time.Now())
	if err != nil {
		t.Fatalf("RehydrateLedgerEntry: %v", err)
	}
	if entry.BalanceAfter().Decimal() != "999.00" {
		t.Fatalf("BalanceAfter = %s, want 999.00", entry.BalanceAfter().Decimal())
	}
}

package domain

import "time"

type EventType string

const (
	EventWagerTransactionProcessed        EventType = "WagerTransactionProcessed"
	EventWagerTransactionRejected         EventType = "WagerTransactionRejected"
	EventWalletBalanceChanged             EventType = "WalletBalanceChanged"
	EventWagerTransactionPendingReference EventType = "WagerTransactionPendingReference"
)

// Event is the envelope persisted as an outbox snapshot. Type and Version
// are fixed by each constructor below, never set by callers.
type Event struct {
	ID            string
	Type          EventType
	AggregateID   string
	CorrelationID string
	CausationID   string
	OccurredAt    time.Time
	Version       int
	Data          any
}

type WagerTransactionProcessedData struct {
	TransactionID string
	WalletID      string
	Kind          TransactionKind
	Direction     MovementDirection
	Amount        Money
	BalanceAfter  Money
}

func NewWagerTransactionProcessedEvent(id string, tx *WagerTransaction, correlationID, causationID string, now time.Time) Event {
	var balanceAfter Money
	if tx.BalanceAfter() != nil {
		balanceAfter = *tx.BalanceAfter()
	}
	return Event{
		ID:            id,
		Type:          EventWagerTransactionProcessed,
		AggregateID:   tx.WalletID(),
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.UTC(),
		Version:       1,
		Data: WagerTransactionProcessedData{
			TransactionID: tx.ID(),
			WalletID:      tx.WalletID(),
			Kind:          tx.Kind(),
			Direction:     tx.Direction(),
			Amount:        tx.Amount(),
			BalanceAfter:  balanceAfter,
		},
	}
}

type WagerTransactionRejectedData struct {
	TransactionID string
	WalletID      string
	Kind          TransactionKind
	FailureCode   FailureCode
}

func NewWagerTransactionRejectedEvent(id string, tx *WagerTransaction, correlationID, causationID string, now time.Time) Event {
	var code FailureCode
	if tx.FailureCode() != nil {
		code = *tx.FailureCode()
	}
	return Event{
		ID:            id,
		Type:          EventWagerTransactionRejected,
		AggregateID:   tx.WalletID(),
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.UTC(),
		Version:       1,
		Data: WagerTransactionRejectedData{
			TransactionID: tx.ID(),
			WalletID:      tx.WalletID(),
			Kind:          tx.Kind(),
			FailureCode:   code,
		},
	}
}

type WalletBalanceChangedData struct {
	WalletID      string
	PlayerID      string
	Balance       Money
	Version       int64
	TransactionID string
}

func NewWalletBalanceChangedEvent(id string, wallet *Wallet, transactionID, correlationID, causationID string, now time.Time) Event {
	return Event{
		ID:            id,
		Type:          EventWalletBalanceChanged,
		AggregateID:   wallet.ID(),
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.UTC(),
		Version:       1,
		Data: WalletBalanceChangedData{
			WalletID:      wallet.ID(),
			PlayerID:      wallet.PlayerID(),
			Balance:       wallet.Balance(),
			Version:       wallet.Version(),
			TransactionID: transactionID,
		},
	}
}

type WagerTransactionPendingReferenceData struct {
	TransactionID          string
	WalletID               string
	Kind                   TransactionKind
	ReferenceTransactionID string
}

func NewWagerTransactionPendingReferenceEvent(id string, tx *WagerTransaction, referenceTransactionID, correlationID, causationID string, now time.Time) Event {
	return Event{
		ID:            id,
		Type:          EventWagerTransactionPendingReference,
		AggregateID:   tx.WalletID(),
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    now.UTC(),
		Version:       1,
		Data: WagerTransactionPendingReferenceData{
			TransactionID:          tx.ID(),
			WalletID:               tx.WalletID(),
			Kind:                   tx.Kind(),
			ReferenceTransactionID: referenceTransactionID,
		},
	}
}

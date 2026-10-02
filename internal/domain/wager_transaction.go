package domain

import (
	"strings"
	"time"
)

type TransactionKind string

const (
	KindOpening  TransactionKind = "OPENING"
	KindBet      TransactionKind = "BET"
	KindWin      TransactionKind = "WIN"
	KindLoss     TransactionKind = "LOSS"
	KindRefund   TransactionKind = "REFUND"
	KindRollback TransactionKind = "ROLLBACK"
)

type TransactionStatus string

const (
	StatusPending          TransactionStatus = "PENDING"
	StatusPendingReference TransactionStatus = "PENDING_REFERENCE"
	StatusProcessed        TransactionStatus = "PROCESSED"
	StatusRejected         TransactionStatus = "REJECTED"
	StatusFailed           TransactionStatus = "FAILED"
)

func (s TransactionStatus) Terminal() bool {
	switch s {
	case StatusProcessed, StatusRejected, StatusFailed:
		return true
	default:
		return false
	}
}

type MovementDirection string

const (
	DirectionNone   MovementDirection = "NONE"
	DirectionDebit  MovementDirection = "DEBIT"
	DirectionCredit MovementDirection = "CREDIT"
)

type WagerTransaction struct {
	id           string
	providerID   string
	playerID     string
	walletID     string
	roundID      string
	gameID       string
	kind         TransactionKind
	status       TransactionStatus
	amount       Money
	direction    MovementDirection
	referenceID  *string
	reversedBy   *string
	failureCode  *FailureCode
	balanceAfter *Money
	createdAt    time.Time
	updatedAt    time.Time
}

func NewWagerTransaction(id, providerID, playerID, walletID, roundID, gameID string, kind TransactionKind, amount Money, now time.Time) (*WagerTransaction, error) {
	if strings.TrimSpace(id) == "" {
		return nil, NewError(FailureInvalidState, "transaction id is required")
	}
	if strings.TrimSpace(playerID) == "" {
		return nil, NewError(FailureInvalidState, "player id is required")
	}
	if strings.TrimSpace(walletID) == "" {
		return nil, NewError(FailureInvalidState, "wallet id is required")
	}

	if kind == KindOpening {
		if providerID != "" || roundID != "" || gameID != "" {
			return nil, NewError(FailureInvalidTransactionKind, "opening transactions must not carry provider, round or game metadata")
		}
	} else {
		if strings.TrimSpace(providerID) == "" || strings.TrimSpace(roundID) == "" || strings.TrimSpace(gameID) == "" {
			return nil, NewError(FailureInvalidTransactionKind, "external transactions require provider, round and game")
		}
	}

	switch kind {
	case KindLoss:
		if !amount.IsZero() {
			return nil, NewError(FailureInvalidAmount, "loss amount must be exactly zero")
		}
	case KindBet, KindWin, KindRefund, KindRollback, KindOpening:
		if !amount.IsPositive() {
			return nil, NewError(FailureInvalidAmount, "amount must be positive")
		}
	default:
		return nil, NewError(FailureInvalidTransactionKind, "unknown transaction kind")
	}

	return &WagerTransaction{
		id:         id,
		providerID: providerID,
		playerID:   playerID,
		walletID:   walletID,
		roundID:    roundID,
		gameID:     gameID,
		kind:       kind,
		amount:     amount,
		status:     StatusPending,
		direction:  DirectionNone,
		createdAt:  now,
		updatedAt:  now,
	}, nil
}

func RehydrateWagerTransaction(
	id, providerID, playerID, walletID, roundID, gameID string,
	kind TransactionKind,
	status TransactionStatus,
	amount Money,
	direction MovementDirection,
	referenceID, reversedBy *string,
	failureCode *FailureCode,
	balanceAfter *Money,
	createdAt, updatedAt time.Time,
) (*WagerTransaction, error) {
	if strings.TrimSpace(id) == "" {
		return nil, NewError(FailureInvalidState, "transaction id is required")
	}

	return &WagerTransaction{
		id:           id,
		providerID:   providerID,
		playerID:     playerID,
		walletID:     walletID,
		roundID:      roundID,
		gameID:       gameID,
		kind:         kind,
		status:       status,
		amount:       amount,
		direction:    direction,
		referenceID:  referenceID,
		reversedBy:   reversedBy,
		failureCode:  failureCode,
		balanceAfter: balanceAfter,
		createdAt:    createdAt,
		updatedAt:    updatedAt,
	}, nil
}

func (tx *WagerTransaction) ID() string                      { return tx.id }
func (tx *WagerTransaction) ProviderID() string              { return tx.providerID }
func (tx *WagerTransaction) PlayerID() string                { return tx.playerID }
func (tx *WagerTransaction) WalletID() string                { return tx.walletID }
func (tx *WagerTransaction) RoundID() string                 { return tx.roundID }
func (tx *WagerTransaction) GameID() string                  { return tx.gameID }
func (tx *WagerTransaction) Kind() TransactionKind           { return tx.kind }
func (tx *WagerTransaction) Status() TransactionStatus       { return tx.status }
func (tx *WagerTransaction) Amount() Money                   { return tx.amount }
func (tx *WagerTransaction) Direction() MovementDirection    { return tx.direction }
func (tx *WagerTransaction) ReferenceTransactionID() *string { return tx.referenceID }
func (tx *WagerTransaction) ReversedBy() *string             { return tx.reversedBy }
func (tx *WagerTransaction) FailureCode() *FailureCode       { return tx.failureCode }
func (tx *WagerTransaction) BalanceAfter() *Money            { return tx.balanceAfter }
func (tx *WagerTransaction) CreatedAt() time.Time            { return tx.createdAt }
func (tx *WagerTransaction) UpdatedAt() time.Time            { return tx.updatedAt }

func (tx *WagerTransaction) requireKind(kind TransactionKind) error {
	if tx.kind != kind {
		return NewError(FailureInvalidTransactionKind, "transaction is not of kind "+string(kind))
	}
	if tx.status.Terminal() {
		return NewError(FailureInvalidState, "transaction is already finalized")
	}
	return nil
}

func (tx *WagerTransaction) finish(direction MovementDirection, balanceAfter Money, now time.Time) {
	tx.direction = direction
	tx.balanceAfter = &balanceAfter
	tx.status = StatusProcessed
	tx.updatedAt = now
}

// The wallet's balance is already set at creation; this only validates and
// snapshots it. Never call this for a zero opening balance.
func (tx *WagerTransaction) ProcessOpening(wallet *Wallet, now time.Time) error {
	if err := tx.requireKind(KindOpening); err != nil {
		return err
	}
	if !tx.amount.IsPositive() {
		return NewError(FailureInvalidAmount, "opening amount must be positive")
	}
	cmp, err := tx.amount.Compare(wallet.Balance())
	if err != nil {
		return err
	}
	if cmp != 0 {
		return NewError(FailureInvalidAmount, "opening amount must match wallet balance")
	}

	tx.finish(DirectionCredit, wallet.Balance(), now)
	return nil
}

func (tx *WagerTransaction) ProcessBet(wallet *Wallet, now time.Time) error {
	if err := tx.requireKind(KindBet); err != nil {
		return err
	}
	if err := wallet.Debit(tx.amount, now); err != nil {
		return err
	}
	tx.finish(DirectionDebit, wallet.Balance(), now)
	return nil
}

func (tx *WagerTransaction) ProcessWin(wallet *Wallet, now time.Time) error {
	if err := tx.requireKind(KindWin); err != nil {
		return err
	}
	if err := wallet.Credit(tx.amount, now); err != nil {
		return err
	}
	tx.finish(DirectionCredit, wallet.Balance(), now)
	return nil
}

func (tx *WagerTransaction) ProcessLoss(wallet *Wallet, now time.Time) error {
	if err := tx.requireKind(KindLoss); err != nil {
		return err
	}
	if !tx.amount.IsZero() {
		return NewError(FailureInvalidAmount, "loss amount must be exactly zero")
	}
	tx.finish(DirectionNone, wallet.Balance(), now)
	return nil
}

func (tx *WagerTransaction) matchesReference(reference *WagerTransaction) error {
	if reference == nil {
		return NewError(FailureReferenceNotFound, "reference transaction is required")
	}
	if tx.providerID != reference.providerID ||
		tx.playerID != reference.playerID ||
		tx.walletID != reference.walletID ||
		tx.roundID != reference.roundID {
		return NewError(FailureReferenceMismatch, "reference transaction does not match provider, player, wallet or round")
	}
	if tx.amount.Currency() != reference.amount.Currency() {
		return NewError(FailureCurrencyMismatch, "reference transaction currency mismatch")
	}
	return nil
}

func (tx *WagerTransaction) checkReversalTarget(reference *WagerTransaction, allowedKinds ...TransactionKind) error {
	if err := tx.matchesReference(reference); err != nil {
		return err
	}

	kindAllowed := false
	for _, kind := range allowedKinds {
		if reference.kind == kind {
			kindAllowed = true
			break
		}
	}
	if !kindAllowed {
		return NewError(FailureReferenceMismatch, "reference transaction kind is not eligible for this reversal")
	}

	switch reference.status {
	case StatusRejected, StatusFailed:
		return NewError(FailureReferenceRejected, "reference transaction was not processed successfully")
	case StatusProcessed:
	default:
		return NewError(FailureReferenceNotProcessed, "reference transaction is not processed")
	}

	if reference.reversedBy != nil {
		return NewError(FailureAlreadyReversed, "reference transaction already has a direct reversal")
	}

	cmp, err := tx.amount.Compare(reference.amount)
	if err != nil {
		return err
	}
	if cmp != 0 {
		return NewError(FailureReferenceMismatch, "reversal amount must match the referenced transaction amount")
	}

	return nil
}

func (tx *WagerTransaction) ProcessRefund(wallet *Wallet, reference *WagerTransaction, now time.Time) error {
	if err := tx.requireKind(KindRefund); err != nil {
		return err
	}
	if err := tx.checkReversalTarget(reference, KindBet); err != nil {
		return err
	}

	if err := wallet.Credit(tx.amount, now); err != nil {
		return err
	}
	if err := reference.markReversed(tx.id, now); err != nil {
		return err
	}

	referenceID := reference.id
	tx.referenceID = &referenceID
	tx.finish(DirectionCredit, wallet.Balance(), now)
	return nil
}

// To undo a REFUND, reference must be the REFUND itself, not the original BET.
func (tx *WagerTransaction) ProcessRollback(wallet *Wallet, reference *WagerTransaction, now time.Time) error {
	if err := tx.requireKind(KindRollback); err != nil {
		return err
	}
	if err := tx.checkReversalTarget(reference, KindBet, KindWin, KindRefund); err != nil {
		return err
	}

	var (
		moveErr   error
		direction MovementDirection
	)
	switch reference.direction {
	case DirectionDebit:
		moveErr = wallet.Credit(tx.amount, now)
		direction = DirectionCredit
	case DirectionCredit:
		moveErr = wallet.Debit(tx.amount, now)
		direction = DirectionDebit
	default:
		return NewError(FailureReferenceMismatch, "referenced transaction has no movement to roll back")
	}
	if moveErr != nil {
		if HasFailureCode(moveErr, FailureInsufficientFunds) {
			return NewError(FailureInsufficientFundsForReversal, "insufficient funds to roll back reference transaction")
		}
		return moveErr
	}

	if err := reference.markReversed(tx.id, now); err != nil {
		return err
	}

	referenceID := reference.id
	tx.referenceID = &referenceID
	tx.finish(direction, wallet.Balance(), now)
	return nil
}

func (tx *WagerTransaction) markReversed(reversalID string, now time.Time) error {
	if tx.reversedBy != nil {
		return NewError(FailureAlreadyReversed, "transaction already has a direct reversal")
	}
	tx.reversedBy = &reversalID
	tx.updatedAt = now
	return nil
}

func (tx *WagerTransaction) MarkPendingReference(now time.Time) error {
	if tx.kind != KindRefund && tx.kind != KindRollback {
		return NewError(FailureInvalidTransactionKind, "only refund or rollback can be pending reference")
	}
	if tx.status.Terminal() {
		return NewError(FailureInvalidState, "transaction is already finalized")
	}
	tx.status = StatusPendingReference
	tx.updatedAt = now
	return nil
}

func (tx *WagerTransaction) MarkRejected(code FailureCode, now time.Time) error {
	if tx.status.Terminal() {
		return NewError(FailureInvalidState, "transaction is already finalized")
	}
	tx.status = StatusRejected
	tx.failureCode = &code
	tx.updatedAt = now
	return nil
}

func (tx *WagerTransaction) MarkFailed(now time.Time) error {
	if tx.status.Terminal() {
		return NewError(FailureInvalidState, "transaction is already finalized")
	}
	tx.status = StatusFailed
	tx.updatedAt = now
	return nil
}

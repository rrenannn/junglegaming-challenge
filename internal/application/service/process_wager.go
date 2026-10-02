package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

// ErrInboxHashConflict is returned when a message with an already-known
// (ConsumerName, MessageID) arrives with different business content than
// what was processed before — a redelivery should never legitimately carry
// different content, so this is treated as invalid and auditable rather
// than silently reprocessed.
var ErrInboxHashConflict = errors.New("inbox message hash conflict")

const maxPendingReferenceAttempts = 10

const (
	pendingReferenceBackoffBase = 10 * time.Second
	pendingReferenceBackoffMax  = 5 * time.Minute
)

// InboxInfo identifies the transport message behind a command, so Execute
// can deduplicate by message id and share the inbox write with the same
// financial commit. Left nil for the HTTP path, which has no message
// envelope.
type InboxInfo struct {
	ConsumerName string
	MessageID    string
}

type ProcessWagerCommand struct {
	TransactionID                  string
	ProviderID                     string
	PlayerID                       string
	WalletID                       string
	RoundID                        string
	GameID                         string
	Kind                           domain.TransactionKind
	Amount                         domain.Money
	ExternalTransactionID          string
	IdempotencyKey                 string
	ReferenceExternalTransactionID string
	Inbox                          *InboxInfo
}

type ProcessWagerResult struct {
	TransactionID    string
	Status           domain.TransactionStatus
	Direction        domain.MovementDirection
	BalanceAfter     domain.Money
	FailureCode      *domain.FailureCode
	AlreadyProcessed bool
}

type ProcessWagerService struct {
	uow   repository.UnitOfWork
	clock port.Clock
	ids   port.IDGenerator
}

func NewProcessWagerService(uow repository.UnitOfWork, clock port.Clock, ids port.IDGenerator) *ProcessWagerService {
	return &ProcessWagerService{uow: uow, clock: clock, ids: ids}
}

func (s *ProcessWagerService) Execute(ctx context.Context, cmd ProcessWagerCommand) (*ProcessWagerResult, error) {
	if cmd.TransactionID == "" {
		cmd.TransactionID = s.ids.NewID()
	}
	if cmd.IdempotencyKey == "" {
		cmd.IdempotencyKey = cmd.ExternalTransactionID
	}

	var result *ProcessWagerResult

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		now := s.clock.Now()
		hash := computeWagerHash(cmd)

		// Transport-level dedup: has this exact SQS message already been
		// handled? Nil for HTTP, which has no message envelope.
		if cmd.Inbox != nil {
			existing, err := repos.Inbox().FindByMessageID(ctx, cmd.Inbox.ConsumerName, cmd.Inbox.MessageID)
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			if err == nil {
				if existing.PayloadHash != hash {
					return ErrInboxHashConflict
				}
				result = &ProcessWagerResult{AlreadyProcessed: true}
				return nil
			}
		}

		markInbox := func() error {
			if cmd.Inbox == nil {
				return nil
			}
			return repos.Inbox().Create(ctx, &repository.InboxMessage{
				ID:           s.ids.NewID(),
				ConsumerName: cmd.Inbox.ConsumerName,
				MessageID:    cmd.Inbox.MessageID,
				PayloadHash:  hash,
				ReceivedAt:   now,
				CompletedAt:  &now,
			})
		}

		// Business-level dedup: has this (provider, idempotency key)
		// already been submitted — by this transport or any other?
		if cmd.IdempotencyKey != "" {
			existingTx, err := repos.Wagers().FindByIdempotencyKey(ctx, cmd.ProviderID, cmd.IdempotencyKey)
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			if err == nil {
				if existingTx.PayloadHash() != hash {
					return repository.ErrIdempotencyKeyConflict
				}
				result = resultFromTransaction(existingTx)
				if err := markInbox(); err != nil {
					return err
				}
				return nil
			}
		}

		wallet, err := repos.Wallets().FindByIDForUpdate(ctx, cmd.WalletID)
		if err != nil {
			return err
		}
		balanceBefore := wallet.Balance()

		tx, err := domain.NewWagerTransaction(domain.NewWagerTransactionParams{
			ID:                             cmd.TransactionID,
			ProviderID:                     cmd.ProviderID,
			PlayerID:                       cmd.PlayerID,
			WalletID:                       cmd.WalletID,
			RoundID:                        cmd.RoundID,
			GameID:                         cmd.GameID,
			Kind:                           cmd.Kind,
			Amount:                         cmd.Amount,
			ExternalTransactionID:          cmd.ExternalTransactionID,
			IdempotencyKey:                 cmd.IdempotencyKey,
			PayloadHash:                    hash,
			ReferenceExternalTransactionID: cmd.ReferenceExternalTransactionID,
		}, now)
		if err != nil {
			return err
		}

		var reference *domain.WagerTransaction
		if cmd.Kind == domain.KindRefund || cmd.Kind == domain.KindRollback {
			reference, err = repos.Wagers().FindByExternalIDForUpdate(ctx, cmd.ProviderID, cmd.ReferenceExternalTransactionID)
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return err
			}

			if reference == nil {
				if err := tx.MarkPendingReference(now); err != nil {
					return err
				}
				if err := repos.Wagers().Create(ctx, tx); err != nil {
					return err
				}
				pendingEvent := domain.NewWagerTransactionPendingReferenceEvent(s.ids.NewID(), tx, cmd.ReferenceExternalTransactionID, "", "", now)
				if err := repos.Outbox().Create(ctx, pendingEvent); err != nil {
					return err
				}
				if err := markInbox(); err != nil {
					return err
				}
				result = &ProcessWagerResult{TransactionID: tx.ID(), Status: tx.Status()}
				return nil
			}
		}

		if processErr := applyWagerKind(tx, wallet, reference, now); processErr != nil {
			var rejectResult *ProcessWagerResult
			rejectResult, err = s.rejectNewTransaction(ctx, repos, tx, processErr, now)
			if err != nil {
				return err
			}
			if err := markInbox(); err != nil {
				return err
			}
			result = rejectResult
			return nil
		}

		if err := repos.Wagers().Create(ctx, tx); err != nil {
			return err
		}
		if err := repos.Wallets().Update(ctx, wallet); err != nil {
			return err
		}
		if reference != nil {
			if err := repos.Wagers().Update(ctx, reference); err != nil {
				return err
			}
		}
		if tx.Direction() != domain.DirectionNone {
			entry, err := domain.NewLedgerEntry(s.ids.NewID(), wallet.ID(), tx.ID(), tx.Direction(), tx.Amount(), balanceBefore, *tx.BalanceAfter(), now)
			if err != nil {
				return err
			}
			if err := repos.Ledger().Create(ctx, entry); err != nil {
				return err
			}
		}

		processedResult, err := s.emitProcessed(ctx, repos, tx, wallet, now)
		if err != nil {
			return err
		}
		if err := markInbox(); err != nil {
			return err
		}
		result = processedResult
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ResolvePendingReference re-attempts resolving a REFUND/ROLLBACK that
// could not find its reference yet. Called by the pending-reference worker,
// never by HTTP/SQS handlers directly.
func (s *ProcessWagerService) ResolvePendingReference(ctx context.Context, transactionID string) (*ProcessWagerResult, error) {
	var result *ProcessWagerResult

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		now := s.clock.Now()

		tx, err := repos.Wagers().FindByIDForUpdate(ctx, transactionID)
		if err != nil {
			return err
		}
		if tx.Status() != domain.StatusPendingReference {
			// Already resolved by a previous attempt or another instance.
			result = &ProcessWagerResult{TransactionID: tx.ID(), Status: tx.Status(), AlreadyProcessed: true}
			return nil
		}

		refExternalID := ""
		if tx.ReferenceExternalTransactionID() != nil {
			refExternalID = *tx.ReferenceExternalTransactionID()
		}
		reference, err := repos.Wagers().FindByExternalIDForUpdate(ctx, tx.ProviderID(), refExternalID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}

		if reference == nil {
			if tx.Attempts() >= maxPendingReferenceAttempts {
				rejectResult, err := s.rejectExistingTransaction(ctx, repos, tx, domain.NewError(domain.FailureReferenceNotFound, "reference transaction was never found"), now)
				if err != nil {
					return err
				}
				result = rejectResult
				return nil
			}

			nextAttempt := now.Add(pendingReferenceBackoff(tx.Attempts()))
			if err := repos.Wagers().ReschedulePendingReference(ctx, tx.ID(), nextAttempt); err != nil {
				return err
			}
			result = &ProcessWagerResult{TransactionID: tx.ID(), Status: tx.Status()}
			return nil
		}

		wallet, err := repos.Wallets().FindByIDForUpdate(ctx, tx.WalletID())
		if err != nil {
			return err
		}
		balanceBefore := wallet.Balance()

		if processErr := applyWagerKind(tx, wallet, reference, now); processErr != nil {
			rejectResult, err := s.rejectExistingTransaction(ctx, repos, tx, processErr, now)
			if err != nil {
				return err
			}
			result = rejectResult
			return nil
		}

		if err := repos.Wagers().Update(ctx, tx); err != nil {
			return err
		}
		if err := repos.Wallets().Update(ctx, wallet); err != nil {
			return err
		}
		if err := repos.Wagers().Update(ctx, reference); err != nil {
			return err
		}
		if tx.Direction() != domain.DirectionNone {
			entry, err := domain.NewLedgerEntry(s.ids.NewID(), wallet.ID(), tx.ID(), tx.Direction(), tx.Amount(), balanceBefore, *tx.BalanceAfter(), now)
			if err != nil {
				return err
			}
			if err := repos.Ledger().Create(ctx, entry); err != nil {
				return err
			}
		}

		processedResult, err := s.emitProcessed(ctx, repos, tx, wallet, now)
		if err != nil {
			return err
		}
		result = processedResult
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// rejectNewTransaction persists tx for the first time, already rejected.
func (s *ProcessWagerService) rejectNewTransaction(ctx context.Context, repos repository.TransactionRepositories, tx *domain.WagerTransaction, processErr error, now time.Time) (*ProcessWagerResult, error) {
	code, err := markRejected(tx, processErr, now)
	if err != nil {
		return nil, err
	}
	if err := repos.Wagers().Create(ctx, tx); err != nil {
		return nil, err
	}
	return s.emitRejected(ctx, repos, tx, code, now)
}

// rejectExistingTransaction transitions an already-persisted (previously
// PENDING_REFERENCE) tx to rejected.
func (s *ProcessWagerService) rejectExistingTransaction(ctx context.Context, repos repository.TransactionRepositories, tx *domain.WagerTransaction, processErr error, now time.Time) (*ProcessWagerResult, error) {
	code, err := markRejected(tx, processErr, now)
	if err != nil {
		return nil, err
	}
	if err := repos.Wagers().Update(ctx, tx); err != nil {
		return nil, err
	}
	return s.emitRejected(ctx, repos, tx, code, now)
}

func markRejected(tx *domain.WagerTransaction, processErr error, now time.Time) (domain.FailureCode, error) {
	code := domain.FailureInvalidState
	var domainErr *domain.Error
	if errors.As(processErr, &domainErr) {
		code = domainErr.Code
	}
	if err := tx.MarkRejected(code, now); err != nil {
		return "", err
	}
	return code, nil
}

func (s *ProcessWagerService) emitRejected(ctx context.Context, repos repository.TransactionRepositories, tx *domain.WagerTransaction, code domain.FailureCode, now time.Time) (*ProcessWagerResult, error) {
	rejectedEvent := domain.NewWagerTransactionRejectedEvent(s.ids.NewID(), tx, "", "", now)
	if err := repos.Outbox().Create(ctx, rejectedEvent); err != nil {
		return nil, err
	}
	return &ProcessWagerResult{TransactionID: tx.ID(), Status: tx.Status(), FailureCode: &code}, nil
}

func (s *ProcessWagerService) emitProcessed(ctx context.Context, repos repository.TransactionRepositories, tx *domain.WagerTransaction, wallet *domain.Wallet, now time.Time) (*ProcessWagerResult, error) {
	processedEvent := domain.NewWagerTransactionProcessedEvent(s.ids.NewID(), tx, "", "", now)
	if err := repos.Outbox().Create(ctx, processedEvent); err != nil {
		return nil, err
	}
	if tx.Direction() != domain.DirectionNone {
		balanceChangedEvent := domain.NewWalletBalanceChangedEvent(s.ids.NewID(), wallet, tx.ID(), "", tx.ID(), now)
		if err := repos.Outbox().Create(ctx, balanceChangedEvent); err != nil {
			return nil, err
		}
	}
	return &ProcessWagerResult{
		TransactionID: tx.ID(),
		Status:        tx.Status(),
		Direction:     tx.Direction(),
		BalanceAfter:  wallet.Balance(),
	}, nil
}

// resultFromTransaction reconstructs a ProcessWagerResult from an already
// persisted transaction, used to replay an idempotent resubmission without
// touching the wallet or the ledger again.
func resultFromTransaction(tx *domain.WagerTransaction) *ProcessWagerResult {
	result := &ProcessWagerResult{
		TransactionID:    tx.ID(),
		Status:           tx.Status(),
		Direction:        tx.Direction(),
		FailureCode:      tx.FailureCode(),
		AlreadyProcessed: true,
	}
	if tx.BalanceAfter() != nil {
		result.BalanceAfter = *tx.BalanceAfter()
	}
	return result
}

func applyWagerKind(tx *domain.WagerTransaction, wallet *domain.Wallet, reference *domain.WagerTransaction, now time.Time) error {
	switch tx.Kind() {
	case domain.KindBet:
		return tx.ProcessBet(wallet, now)
	case domain.KindWin:
		return tx.ProcessWin(wallet, now)
	case domain.KindLoss:
		return tx.ProcessLoss(wallet, now)
	case domain.KindRefund:
		return tx.ProcessRefund(wallet, reference, now)
	case domain.KindRollback:
		return tx.ProcessRollback(wallet, reference, now)
	default:
		return domain.NewError(domain.FailureInvalidTransactionKind, "unsupported transaction kind")
	}
}

// pendingReferenceBackoff grows exponentially with attempts already made,
// capped at pendingReferenceBackoffMax, with up to 20% jitter so concurrent
// workers don't retry in lockstep. Same shape as the outbox publisher's
// backoff (internal/adapter/sqs/worker/outbox_publisher.go); not shared
// since it is little logic duplicated for two unrelated retry loops.
func pendingReferenceBackoff(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	shift := attempts
	if shift > 20 {
		shift = 20
	}
	delay := pendingReferenceBackoffBase * time.Duration(1<<shift)
	if delay <= 0 || delay > pendingReferenceBackoffMax {
		delay = pendingReferenceBackoffMax
	}
	jitter := time.Duration(rand.Int63n(int64(delay)/5 + 1))
	return delay + jitter
}

// computeWagerHash hashes the business fields of a command so HTTP and SQS
// produce identical results for the same operation. It intentionally
// excludes transport metadata (transaction id, idempotency key itself).
func computeWagerHash(cmd ProcessWagerCommand) string {
	type wagerHashPayload struct {
		ProviderID                     string
		ExternalTransactionID          string
		PlayerID                       string
		WalletID                       string
		RoundID                        string
		GameID                         string
		Kind                           string
		AmountDecimal                  string
		Currency                       string
		ReferenceExternalTransactionID string
	}

	payload := wagerHashPayload{
		ProviderID:                     cmd.ProviderID,
		ExternalTransactionID:          cmd.ExternalTransactionID,
		PlayerID:                       cmd.PlayerID,
		WalletID:                       cmd.WalletID,
		RoundID:                        cmd.RoundID,
		GameID:                         cmd.GameID,
		Kind:                           string(cmd.Kind),
		AmountDecimal:                  cmd.Amount.Decimal(),
		Currency:                       string(cmd.Amount.Currency()),
		ReferenceExternalTransactionID: cmd.ReferenceExternalTransactionID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		panic("compute wager hash: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// InboxInfo identifies the transport message behind a command, so Execute
// can deduplicate by message id and share the inbox write with the same
// financial commit. Left nil for the HTTP path, which has no message
// envelope.
type InboxInfo struct {
	ConsumerName string
	MessageID    string
}

type ProcessWagerCommand struct {
	TransactionID          string
	ProviderID             string
	PlayerID               string
	WalletID               string
	RoundID                string
	GameID                 string
	Kind                   domain.TransactionKind
	Amount                 domain.Money
	ReferenceTransactionID string
	Inbox                  *InboxInfo
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

	var result *ProcessWagerResult

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		now := s.clock.Now()

		var inboxHash string
		if cmd.Inbox != nil {
			inboxHash = computeWagerHash(cmd)
			existing, err := repos.Inbox().FindByMessageID(ctx, cmd.Inbox.ConsumerName, cmd.Inbox.MessageID)
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			if err == nil {
				if existing.PayloadHash != inboxHash {
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
				PayloadHash:  inboxHash,
				ReceivedAt:   now,
				CompletedAt:  &now,
			})
		}

		wallet, err := repos.Wallets().FindByIDForUpdate(ctx, cmd.WalletID)
		if err != nil {
			return err
		}
		balanceBefore := wallet.Balance()

		tx, err := domain.NewWagerTransaction(cmd.TransactionID, cmd.ProviderID, cmd.PlayerID, cmd.WalletID, cmd.RoundID, cmd.GameID, cmd.Kind, cmd.Amount, now)
		if err != nil {
			return err
		}

		var reference *domain.WagerTransaction
		if cmd.Kind == domain.KindRefund || cmd.Kind == domain.KindRollback {
			reference, err = repos.Wagers().FindByIDForUpdate(ctx, cmd.ReferenceTransactionID)
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return err
			}
		}

		if processErr := applyWagerKind(tx, wallet, reference, now); processErr != nil {
			code := domain.FailureInvalidState
			var domainErr *domain.Error
			if errors.As(processErr, &domainErr) {
				code = domainErr.Code
			}
			if err := tx.MarkRejected(code, now); err != nil {
				return err
			}
			if err := repos.Wagers().Create(ctx, tx); err != nil {
				return err
			}
			rejectedEvent := domain.NewWagerTransactionRejectedEvent(s.ids.NewID(), tx, "", "", now)
			if err := repos.Outbox().Create(ctx, rejectedEvent); err != nil {
				return err
			}
			if err := markInbox(); err != nil {
				return err
			}
			result = &ProcessWagerResult{TransactionID: tx.ID(), Status: tx.Status(), FailureCode: &code}
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

		processedEvent := domain.NewWagerTransactionProcessedEvent(s.ids.NewID(), tx, "", "", now)
		if err := repos.Outbox().Create(ctx, processedEvent); err != nil {
			return err
		}
		if tx.Direction() != domain.DirectionNone {
			balanceChangedEvent := domain.NewWalletBalanceChangedEvent(s.ids.NewID(), wallet, tx.ID(), "", tx.ID(), now)
			if err := repos.Outbox().Create(ctx, balanceChangedEvent); err != nil {
				return err
			}
		}

		if err := markInbox(); err != nil {
			return err
		}

		result = &ProcessWagerResult{
			TransactionID: tx.ID(),
			Status:        tx.Status(),
			Direction:     tx.Direction(),
			BalanceAfter:  wallet.Balance(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
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

// computeWagerHash hashes the business fields of a command so HTTP and SQS
// produce identical results for the same operation. It intentionally
// excludes transport metadata (idempotency key, transaction ID) and the
// external transaction ID, which does not exist in the command yet.
func computeWagerHash(cmd ProcessWagerCommand) string {
	type wagerHashPayload struct {
		ProviderID             string
		PlayerID               string
		WalletID               string
		RoundID                string
		GameID                 string
		Kind                   string
		AmountDecimal          string
		Currency               string
		ReferenceTransactionID string
	}

	payload := wagerHashPayload{
		ProviderID:             cmd.ProviderID,
		PlayerID:               cmd.PlayerID,
		WalletID:               cmd.WalletID,
		RoundID:                cmd.RoundID,
		GameID:                 cmd.GameID,
		Kind:                   string(cmd.Kind),
		AmountDecimal:          cmd.Amount.Decimal(),
		Currency:               string(cmd.Amount.Currency()),
		ReferenceTransactionID: cmd.ReferenceTransactionID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		panic("compute wager hash: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

package service

import (
	"context"
	"errors"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

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
}

type ProcessWagerResult struct {
	TransactionID string
	Status        domain.TransactionStatus
	Direction     domain.MovementDirection
	BalanceAfter  domain.Money
	FailureCode   *domain.FailureCode
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
	var result *ProcessWagerResult

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		now := s.clock.Now()

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

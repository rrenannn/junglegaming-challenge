package service

import (
	"context"
	"strings"

	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type OpenWalletCommand struct {
	PlayerID       string
	Currency       string
	OpeningBalance string
}

type OpenWalletService struct {
	uow   repository.UnitOfWork
	clock port.Clock
	ids   port.IDGenerator
}

func NewOpenWalletService(uow repository.UnitOfWork, clock port.Clock, ids port.IDGenerator) *OpenWalletService {
	return &OpenWalletService{uow: uow, clock: clock, ids: ids}
}

func (s *OpenWalletService) Execute(ctx context.Context, cmd OpenWalletCommand) (*domain.Wallet, error) {
	currency, err := domain.NewCurrency(cmd.Currency)
	if err != nil {
		return nil, err
	}

	decimalBalance := strings.TrimSpace(cmd.OpeningBalance)
	if decimalBalance == "" {
		decimalBalance = "0.00"
	}
	openingBalance, err := domain.ParseMoney(decimalBalance, currency)
	if err != nil {
		return nil, err
	}

	var result *domain.Wallet

	err = s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		now := s.clock.Now()

		wallet, err := domain.NewWallet(s.ids.NewID(), cmd.PlayerID, openingBalance, now)
		if err != nil {
			return err
		}

		if err := repos.Wallets().Create(ctx, wallet); err != nil {
			return err
		}

		if openingBalance.IsPositive() {
			tx, err := domain.NewWagerTransaction(s.ids.NewID(), "", cmd.PlayerID, wallet.ID(), "", "", domain.KindOpening, openingBalance, now)
			if err != nil {
				return err
			}
			if err := tx.ProcessOpening(wallet, now); err != nil {
				return err
			}
			if err := repos.Wagers().Create(ctx, tx); err != nil {
				return err
			}

			zero, err := domain.ZeroMoney(currency)
			if err != nil {
				return err
			}
			entry, err := domain.NewLedgerEntry(s.ids.NewID(), wallet.ID(), tx.ID(), domain.DirectionCredit, openingBalance, zero, wallet.Balance(), now)
			if err != nil {
				return err
			}
			if err := repos.Ledger().Create(ctx, entry); err != nil {
				return err
			}

			event := domain.NewWalletBalanceChangedEvent(s.ids.NewID(), wallet, tx.ID(), "", tx.ID(), now)
			if err := repos.Outbox().Create(ctx, event); err != nil {
				return err
			}
		}

		result = wallet
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

package service

import (
	"context"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type ReconciliationResult struct {
	WalletID        string
	RecordedBalance domain.Money
	ComputedBalance domain.Money
	Balanced        bool
	Divergence      *domain.Money
}

type ReconcileWalletService struct {
	uow repository.UnitOfWork
}

func NewReconcileWalletService(uow repository.UnitOfWork) *ReconcileWalletService {
	return &ReconcileWalletService{uow: uow}
}

func (s *ReconcileWalletService) Execute(ctx context.Context, walletID string) (*ReconciliationResult, error) {
	var result *ReconciliationResult

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		wallet, err := repos.Wallets().FindByIDForUpdate(ctx, walletID)
		if err != nil {
			return err
		}

		computed, err := repos.Ledger().SumByWallet(ctx, walletID, wallet.Currency())
		if err != nil {
			return err
		}

		cmp, err := wallet.Balance().Compare(computed)
		if err != nil {
			return err
		}

		result = &ReconciliationResult{
			WalletID:        walletID,
			RecordedBalance: wallet.Balance(),
			ComputedBalance: computed,
			Balanced:        cmp == 0,
		}
		if cmp != 0 {
			divergence, err := wallet.Balance().Sub(computed)
			if err != nil {
				return err
			}
			result.Divergence = &divergence
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

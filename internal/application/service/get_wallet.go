package service

import (
	"context"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type GetWalletService struct {
	uow repository.UnitOfWork
}

func NewGetWalletService(uow repository.UnitOfWork) *GetWalletService {
	return &GetWalletService{uow: uow}
}

func (s *GetWalletService) Execute(ctx context.Context, walletID string) (*domain.Wallet, error) {
	var result *domain.Wallet

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		wallet, err := repos.Wallets().FindByID(ctx, walletID)
		if err != nil {
			return err
		}
		result = wallet
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

package service

import (
	"context"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type GetTransactionQuery struct {
	TransactionID       string
	RequesterProviderID string
}

type GetTransactionService struct {
	uow repository.UnitOfWork
}

func NewGetTransactionService(uow repository.UnitOfWork) *GetTransactionService {
	return &GetTransactionService{uow: uow}
}

func (s *GetTransactionService) Execute(ctx context.Context, query GetTransactionQuery) (*domain.WagerTransaction, error) {
	var result *domain.WagerTransaction

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		tx, err := repos.Wagers().FindByID(ctx, query.TransactionID)
		if err != nil {
			return err
		}
		if query.RequesterProviderID != "" && tx.ProviderID() != query.RequesterProviderID {
			return repository.ErrNotFound
		}
		result = tx
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

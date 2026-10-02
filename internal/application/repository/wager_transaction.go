package repository

import (
	"context"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type WagerTransactionRepository interface {
	Create(ctx context.Context, tx *domain.WagerTransaction) error
	Update(ctx context.Context, tx *domain.WagerTransaction) error
	FindByIDForUpdate(ctx context.Context, id string) (*domain.WagerTransaction, error)
}

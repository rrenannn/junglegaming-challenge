package repository

import (
	"context"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type WalletRepository interface {
	Create(ctx context.Context, wallet *domain.Wallet) error
	Update(ctx context.Context, wallet *domain.Wallet) error
	FindByIDForUpdate(ctx context.Context, id string) (*domain.Wallet, error)
}

package repository

import (
	"context"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type WagerTransactionRepository interface {
	Create(ctx context.Context, tx *domain.WagerTransaction) error
	Update(ctx context.Context, tx *domain.WagerTransaction) error
	FindByID(ctx context.Context, id string) (*domain.WagerTransaction, error)
	FindByIDForUpdate(ctx context.Context, id string) (*domain.WagerTransaction, error)
	FindByExternalIDForUpdate(ctx context.Context, providerID, externalTransactionID string) (*domain.WagerTransaction, error)
	FindByIdempotencyKey(ctx context.Context, providerID, idempotencyKey string) (*domain.WagerTransaction, error)
	FindDuePendingReferences(ctx context.Context, limit int) ([]*domain.WagerTransaction, error)
	ReschedulePendingReference(ctx context.Context, id string, nextAttemptAt time.Time) error
}

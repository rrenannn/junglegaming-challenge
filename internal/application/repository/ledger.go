package repository

import (
	"context"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type LedgerRepository interface {
	Create(ctx context.Context, entry *domain.LedgerEntry) error
}

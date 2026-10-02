package repository

import (
	"context"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type LedgerCursor struct {
	CreatedAt time.Time
	ID        string
}

type LedgerRepository interface {
	Create(ctx context.Context, entry *domain.LedgerEntry) error
	ListByWallet(ctx context.Context, walletID string, after *LedgerCursor, limit int) ([]*domain.LedgerEntry, error)
}

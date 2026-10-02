package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

const (
	defaultLedgerPageSize = 50
	maxLedgerPageSize     = 200
)

var ErrInvalidCursor = errors.New("invalid cursor")

type ListLedgerQuery struct {
	WalletID string
	Cursor   string
	Limit    int
}

type LedgerPage struct {
	Entries    []*domain.LedgerEntry
	NextCursor string
}

type ListLedgerService struct {
	uow repository.UnitOfWork
}

func NewListLedgerService(uow repository.UnitOfWork) *ListLedgerService {
	return &ListLedgerService{uow: uow}
}

func (s *ListLedgerService) Execute(ctx context.Context, query ListLedgerQuery) (*LedgerPage, error) {
	limit := query.Limit
	if limit <= 0 || limit > maxLedgerPageSize {
		limit = defaultLedgerPageSize
	}

	after, err := decodeLedgerCursor(query.Cursor)
	if err != nil {
		return nil, err
	}

	var page *LedgerPage

	err = s.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		if _, err := repos.Wallets().FindByID(ctx, query.WalletID); err != nil {
			return err
		}

		entries, err := repos.Ledger().ListByWallet(ctx, query.WalletID, after, limit+1)
		if err != nil {
			return err
		}

		hasMore := len(entries) > limit
		if hasMore {
			entries = entries[:limit]
		}

		nextCursor := ""
		if hasMore {
			last := entries[len(entries)-1]
			nextCursor = encodeLedgerCursor(last.CreatedAt(), last.ID())
		}

		page = &LedgerPage{Entries: entries, NextCursor: nextCursor}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return page, nil
}

type ledgerCursorPayload struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

func encodeLedgerCursor(createdAt time.Time, id string) string {
	data, err := json.Marshal(ledgerCursorPayload{CreatedAt: createdAt, ID: id})
	if err != nil {
		panic("encode ledger cursor: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeLedgerCursor(cursor string) (*repository.LedgerCursor, error) {
	if cursor == "" {
		return nil, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	var payload ledgerCursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, ErrInvalidCursor
	}
	if payload.ID == "" || payload.CreatedAt.IsZero() {
		return nil, ErrInvalidCursor
	}

	return &repository.LedgerCursor{CreatedAt: payload.CreatedAt, ID: payload.ID}, nil
}

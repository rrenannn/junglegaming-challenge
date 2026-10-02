package repository

import "context"

type TransactionRepositories interface {
	Wallets() WalletRepository
	Wagers() WagerTransactionRepository
	Ledger() LedgerRepository
	Inbox() InboxRepository
	Outbox() OutboxRepository
}

type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(context.Context, TransactionRepositories) error) error
}

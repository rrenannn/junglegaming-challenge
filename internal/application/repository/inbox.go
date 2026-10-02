package repository

import (
	"context"
	"time"
)

type InboxMessage struct {
	ID           string
	ConsumerName string
	MessageID    string
	PayloadHash  string
	ReceivedAt   time.Time
	CompletedAt  *time.Time
}

type InboxRepository interface {
	Create(ctx context.Context, msg *InboxMessage) error
	FindByMessageID(ctx context.Context, consumerName, messageID string) (*InboxMessage, error)
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
)

type InboxRepository struct {
	tx pgx.Tx
}

func NewInboxRepository(tx pgx.Tx) *InboxRepository {
	return &InboxRepository{tx: tx}
}

func (r *InboxRepository) Create(ctx context.Context, msg *repository.InboxMessage) error {
	_, err := r.tx.Exec(ctx, `
		INSERT INTO inbox_messages (id, consumer_name, message_id, payload_hash, received_at, completed_at)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, msg.ID, msg.ConsumerName, msg.MessageID, msg.PayloadHash, msg.ReceivedAt, msg.CompletedAt)
	if err != nil {
		return fmt.Errorf("create inbox message: %w", err)
	}
	return nil
}

func (r *InboxRepository) FindByMessageID(ctx context.Context, consumerName, messageID string) (*repository.InboxMessage, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT id, consumer_name, message_id, payload_hash, received_at, completed_at
		FROM inbox_messages WHERE consumer_name = $1 AND message_id = $2
	`, consumerName, messageID)

	var msg repository.InboxMessage
	var completedAt *time.Time
	if err := row.Scan(&msg.ID, &msg.ConsumerName, &msg.MessageID, &msg.PayloadHash, &msg.ReceivedAt, &completedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("find inbox message: %w", err)
	}
	msg.CompletedAt = completedAt
	return &msg, nil
}

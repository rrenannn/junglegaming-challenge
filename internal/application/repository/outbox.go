package repository

import (
	"context"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type OutboxEvent struct {
	ID          string
	AggregateID string
	EventType   string
	Payload     []byte
	OccurredAt  time.Time
	Attempts    int
}

type OutboxRepository interface {
	Create(ctx context.Context, event domain.Event) error
	ClaimBatch(ctx context.Context, workerID string, leaseDuration time.Duration, limit int) ([]*OutboxEvent, error)
	MarkPublished(ctx context.Context, id string, publishedAt time.Time) error
	ReleaseForRetry(ctx context.Context, id string, nextAttemptAt time.Time) error
}

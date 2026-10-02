package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type OutboxRepository struct {
	tx pgx.Tx
}

func NewOutboxRepository(tx pgx.Tx) *OutboxRepository {
	return &OutboxRepository{tx: tx}
}

func (r *OutboxRepository) Create(ctx context.Context, event domain.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal outbox event: %w", err)
	}

	// next_attempt_at is left to its DEFAULT now() (set by Postgres, not Go)
	// so it is always comparable with the now() used by ClaimBatch on the
	// same server, with no risk of clock skew between the app host and the
	// database making a just-inserted event look not-yet-due.
	_, err = r.tx.Exec(ctx, `
		INSERT INTO outbox_events (id, aggregate_id, event_type, payload, occurred_at)
		VALUES ($1,$2,$3,$4,$5)
	`, event.ID, event.AggregateID, string(event.Type), payload, event.OccurredAt)
	if err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}
	return nil
}

func (r *OutboxRepository) ClaimBatch(ctx context.Context, workerID string, leaseDuration time.Duration, limit int) ([]*repository.OutboxEvent, error) {
	rows, err := r.tx.Query(ctx, `
		UPDATE outbox_events
		SET locked_by = $1, locked_until = now() + ($2 * interval '1 second'), attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM outbox_events
			WHERE published_at IS NULL
				AND next_attempt_at <= now()
				AND (locked_until IS NULL OR locked_until < now())
			ORDER BY next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		RETURNING id, aggregate_id, event_type, payload, occurred_at, attempts
	`, workerID, leaseDuration.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox batch: %w", err)
	}
	defer rows.Close()

	var events []*repository.OutboxEvent
	for rows.Next() {
		var event repository.OutboxEvent
		if err := rows.Scan(&event.ID, &event.AggregateID, &event.EventType, &event.Payload, &event.OccurredAt, &event.Attempts); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox events: %w", err)
	}
	return events, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	tag, err := r.tx.Exec(ctx, `
		UPDATE outbox_events SET published_at = $1, locked_by = NULL, locked_until = NULL WHERE id = $2
	`, publishedAt, id)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *OutboxRepository) ReleaseForRetry(ctx context.Context, id string, nextAttemptAt time.Time) error {
	tag, err := r.tx.Exec(ctx, `
		UPDATE outbox_events SET locked_by = NULL, locked_until = NULL, next_attempt_at = $1 WHERE id = $2
	`, nextAttemptAt, id)
	if err != nil {
		return fmt.Errorf("release outbox event for retry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

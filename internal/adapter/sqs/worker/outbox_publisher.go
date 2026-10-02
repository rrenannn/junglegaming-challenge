package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"

	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
)

const (
	pollInterval  = 2 * time.Second
	batchSize     = 20
	leaseDuration = 30 * time.Second

	backoffBase = time.Second
	backoffMax  = 5 * time.Minute
)

type OutboxPublisher struct {
	uow       repository.UnitOfWork
	client    *sqs.Client
	queueName string
	queueURL  string
	workerID  string
	logger    *slog.Logger
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewOutboxPublisher(uow repository.UnitOfWork, client *sqs.Client, cfg config.Config, logger *slog.Logger) *OutboxPublisher {
	return &OutboxPublisher{
		uow:       uow,
		client:    client,
		queueName: cfg.SQS.EventsQueue,
		workerID:  uuid.NewString(),
		logger:    logger,
	}
}

func (p *OutboxPublisher) Start(ctx context.Context) error {
	out, err := p.client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(p.queueName)})
	if err != nil {
		return fmt.Errorf("resolve events queue url: %w", err)
	}
	p.queueURL = aws.ToString(out.QueueUrl)

	runCtx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})

	p.logger.Info("outbox publisher starting", "queue", p.queueName, "workerId", p.workerID)
	go p.run(runCtx)
	return nil
}

func (p *OutboxPublisher) Shutdown(ctx context.Context) error {
	if p.cancel == nil {
		return nil
	}
	p.logger.Info("outbox publisher shutting down")
	p.cancel()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *OutboxPublisher) run(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.publishBatch(ctx)
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) {
	var claimed []*repository.OutboxEvent
	err := p.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		var err error
		claimed, err = repos.Outbox().ClaimBatch(ctx, p.workerID, leaseDuration, batchSize)
		return err
	})
	if err != nil {
		p.logger.Error("claim outbox batch", "error", err)
		return
	}

	for _, event := range claimed {
		p.publishOne(ctx, event)
	}
}

func (p *OutboxPublisher) publishOne(ctx context.Context, event *repository.OutboxEvent) {
	_, err := p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(p.queueURL),
		MessageBody:            aws.String(string(event.Payload)),
		MessageGroupId:         aws.String(event.AggregateID),
		MessageDeduplicationId: aws.String(event.ID),
	})
	if err != nil {
		p.logger.Error("publish outbox event", "eventId", event.ID, "error", err)
		nextAttempt := time.Now().Add(outboxBackoff(event.Attempts))
		releaseErr := p.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
			return repos.Outbox().ReleaseForRetry(ctx, event.ID, nextAttempt)
		})
		if releaseErr != nil {
			p.logger.Error("release outbox event for retry", "eventId", event.ID, "error", releaseErr)
		}
		return
	}

	markErr := p.uow.WithinTransaction(ctx, func(ctx context.Context, repos repository.TransactionRepositories) error {
		return repos.Outbox().MarkPublished(ctx, event.ID, time.Now())
	})
	if markErr != nil {
		// The event was already sent to SQS. If we crash or fail to record
		// that here, the lease eventually expires and the event gets
		// reclaimed and republished with the same eventId — at-least-once,
		// the downstream consumer must deduplicate by eventId.
		p.logger.Error("mark outbox event published", "eventId", event.ID, "error", markErr)
	}
}

// outboxBackoff grows exponentially with the number of attempts already
// made (including the one that just failed), capped at backoffMax, with up
// to 20% jitter to avoid publishers retrying in lockstep.
func outboxBackoff(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	shift := attempts
	if shift > 20 {
		shift = 20
	}
	delay := backoffBase * time.Duration(1<<shift)
	if delay <= 0 || delay > backoffMax {
		delay = backoffMax
	}
	jitter := time.Duration(rand.Int63n(int64(delay)/5 + 1))
	return delay + jitter
}

package sqsadapter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/sqs/handler"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/observability"
)

type Consumer struct {
	client    *sqs.Client
	queueName string
	queueURL  string
	handler   *handler.Wager
	logger    *slog.Logger
	metrics   *observability.Metrics
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewConsumer(client *sqs.Client, cfg config.Config, wagerHandler *handler.Wager, logger *slog.Logger, metrics *observability.Metrics) *Consumer {
	return &Consumer{
		client:    client,
		queueName: cfg.SQS.InputQueue,
		handler:   wagerHandler,
		logger:    logger,
		metrics:   metrics,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	out, err := c.client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(c.queueName)})
	if err != nil {
		return fmt.Errorf("resolve input queue url: %w", err)
	}
	c.queueURL = aws.ToString(out.QueueUrl)

	runCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})

	c.logger.Info("SQS consumer starting", "queue", c.queueName)
	go c.run(runCtx)
	return nil
}

func (c *Consumer) Shutdown(ctx context.Context) error {
	if c.cancel == nil {
		return nil
	}
	c.logger.Info("SQS consumer shutting down")
	c.cancel()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Consumer) run(ctx context.Context) {
	defer close(c.done)
	for {
		if ctx.Err() != nil {
			return
		}

		out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(c.queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.Error("receive message", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, msg := range out.Messages {
			c.processOne(ctx, msg)
		}
	}
}

func (c *Consumer) processOne(ctx context.Context, msg types.Message) {
	if err := c.handler.Handle(ctx, msg); err != nil {
		c.metrics.ObserveSQSMessage("error")
		c.logger.Error("process wager message", "messageId", aws.ToString(msg.MessageId), "error", err)
		return
	}
	c.metrics.ObserveSQSMessage("success")

	if _, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	}); err != nil {
		c.logger.Error("delete message", "messageId", aws.ToString(msg.MessageId), "error", err)
	}
}

package messaging

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
)

func NewSQSClient(cfg config.Config) (*sqs.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(cfg.SQS.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.SQS.AccessKeyID,
			cfg.SQS.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	client := sqs.NewFromConfig(awsCfg, func(options *sqs.Options) {
		options.BaseEndpoint = aws.String(cfg.SQS.Endpoint)
	})
	return client, nil
}

type SQSHealthCheck struct {
	client *sqs.Client
	queues []string
}

func NewSQSHealthCheck(client *sqs.Client, cfg config.Config) *SQSHealthCheck {
	return &SQSHealthCheck{
		client: client,
		queues: []string{cfg.SQS.InputQueue, cfg.SQS.DLQ, cfg.SQS.EventsQueue},
	}
}

func (h *SQSHealthCheck) Name() string {
	return "sqs"
}

func (h *SQSHealthCheck) Check(ctx context.Context) error {
	for _, queue := range h.queues {
		if _, err := h.client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(queue)}); err != nil {
			return fmt.Errorf("get queue %s: %w", queue, err)
		}
	}
	return nil
}

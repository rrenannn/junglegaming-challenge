#!/bin/sh
set -eu

REGION="${AWS_DEFAULT_REGION:-us-east-1}"

DLQ_URL="$(awslocal sqs create-queue \
  --region "$REGION" \
  --queue-name wager-transactions-dlq.fifo \
  --attributes FifoQueue=true,ContentBasedDeduplication=false \
  --query QueueUrl \
  --output text)"

DLQ_ARN="$(awslocal sqs get-queue-attributes \
  --region "$REGION" \
  --queue-url "$DLQ_URL" \
  --attribute-names QueueArn \
  --query 'Attributes.QueueArn' \
  --output text)"

awslocal sqs create-queue \
  --region "$REGION" \
  --queue-name wager-transactions.fifo \
  --attributes "{\"FifoQueue\":\"true\",\"ContentBasedDeduplication\":\"false\",\"VisibilityTimeout\":\"30\",\"ReceiveMessageWaitTimeSeconds\":\"20\",\"RedrivePolicy\":\"{\\\"deadLetterTargetArn\\\":\\\"${DLQ_ARN}\\\",\\\"maxReceiveCount\\\":\\\"5\\\"}\"}"

awslocal sqs create-queue \
  --region "$REGION" \
  --queue-name wager-events.fifo \
  --attributes '{"FifoQueue":"true","ContentBasedDeduplication":"false","VisibilityTimeout":"30","ReceiveMessageWaitTimeSeconds":"20"}'

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/rrenannn/junglegaming-challenge/internal/application/service"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

const ConsumerName = "wager-transactions"

type Wager struct {
	process *service.ProcessWagerService
	logger  *slog.Logger
}

func NewWager(process *service.ProcessWagerService, logger *slog.Logger) *Wager {
	return &Wager{process: process, logger: logger}
}

type wagerMessage struct {
	ProviderID             string `json:"providerId"`
	PlayerID               string `json:"playerId"`
	WalletID               string `json:"walletId"`
	RoundID                string `json:"roundId"`
	GameID                 string `json:"gameId"`
	Kind                   string `json:"kind"`
	Currency               string `json:"currency"`
	Amount                 string `json:"amount"`
	ReferenceTransactionID string `json:"referenceTransactionId,omitempty"`
}

// Handle processes one SQS message. A nil error means the message can be
// deleted from the queue (processed, rejected by a business rule, or a safe
// duplicate of an already-processed message). A non-nil error means the
// message must be left in the queue for SQS to redeliver or, after enough
// attempts, move to the DLQ.
func (h *Wager) Handle(ctx context.Context, msg types.Message) error {
	var body wagerMessage
	if err := json.Unmarshal([]byte(aws.ToString(msg.Body)), &body); err != nil {
		return fmt.Errorf("invalid message envelope: %w", err)
	}

	currency, err := domain.NewCurrency(body.Currency)
	if err != nil {
		return err
	}
	amount, err := domain.ParseMoney(body.Amount, currency)
	if err != nil {
		return err
	}

	result, err := h.process.Execute(ctx, service.ProcessWagerCommand{
		ProviderID:             body.ProviderID,
		PlayerID:               body.PlayerID,
		WalletID:               body.WalletID,
		RoundID:                body.RoundID,
		GameID:                 body.GameID,
		Kind:                   domain.TransactionKind(body.Kind),
		Amount:                 amount,
		ReferenceTransactionID: body.ReferenceTransactionID,
		Inbox: &service.InboxInfo{
			ConsumerName: ConsumerName,
			MessageID:    aws.ToString(msg.MessageId),
		},
	})
	if err != nil {
		return err
	}

	h.logger.InfoContext(ctx, "wager message processed",
		"messageId", aws.ToString(msg.MessageId),
		"status", result.Status,
		"alreadyProcessed", result.AlreadyProcessed,
	)
	return nil
}

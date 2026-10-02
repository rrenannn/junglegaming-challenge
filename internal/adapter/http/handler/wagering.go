package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/middleware"
	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/response"
	"github.com/rrenannn/junglegaming-challenge/internal/application/service"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type Wagering struct {
	process *service.ProcessWagerService
	get     *service.GetTransactionService
}

func NewWagering(process *service.ProcessWagerService, get *service.GetTransactionService) *Wagering {
	return &Wagering{process: process, get: get}
}

type submitWagerRequest struct {
	ProviderID                     string `json:"providerId"`
	PlayerID                       string `json:"playerId"`
	WalletID                       string `json:"walletId"`
	RoundID                        string `json:"roundId"`
	GameID                         string `json:"gameId"`
	Kind                           string `json:"kind"`
	Currency                       string `json:"currency"`
	Amount                         string `json:"amount"`
	ExternalTransactionID          string `json:"externalTransactionId"`
	IdempotencyKey                 string `json:"idempotencyKey,omitempty"`
	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId,omitempty"`
}

type submitWagerResponse struct {
	TransactionID string        `json:"transactionId"`
	Status        string        `json:"status"`
	Direction     string        `json:"direction"`
	BalanceAfter  *domain.Money `json:"balanceAfter,omitempty"`
	FailureCode   *string       `json:"failureCode,omitempty"`
}

func (h *Wagering) Submit(w http.ResponseWriter, r *http.Request) {
	var req submitWagerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}
	if req.ProviderID == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "providerId is required")
		return
	}

	identity, _ := middleware.IdentityFromContext(r.Context())
	if identity.ProviderID != req.ProviderID {
		response.Error(w, http.StatusForbidden, "FORBIDDEN", "providerId does not match authenticated provider")
		return
	}

	currency, err := domain.NewCurrency(req.Currency)
	if err != nil {
		writeServiceError(w, err, "WAGER_TRANSACTION_NOT_FOUND", "wager transaction not found")
		return
	}
	amount, err := domain.ParseMoney(req.Amount, currency)
	if err != nil {
		writeServiceError(w, err, "WAGER_TRANSACTION_NOT_FOUND", "wager transaction not found")
		return
	}

	result, err := h.process.Execute(r.Context(), service.ProcessWagerCommand{
		ProviderID:                     req.ProviderID,
		PlayerID:                       req.PlayerID,
		WalletID:                       req.WalletID,
		RoundID:                        req.RoundID,
		GameID:                         req.GameID,
		Kind:                           domain.TransactionKind(req.Kind),
		Amount:                         amount,
		ExternalTransactionID:          req.ExternalTransactionID,
		IdempotencyKey:                 req.IdempotencyKey,
		ReferenceExternalTransactionID: req.ReferenceExternalTransactionID,
	})
	if err != nil {
		writeServiceError(w, err, "WAGER_TRANSACTION_NOT_FOUND", "wager transaction not found")
		return
	}

	status := http.StatusOK
	switch result.Status {
	case domain.StatusProcessed:
		status = http.StatusOK
	case domain.StatusPendingReference:
		status = http.StatusAccepted
	case domain.StatusRejected:
		status = http.StatusUnprocessableEntity
	}
	response.JSON(w, status, toSubmitWagerResponse(result))
}

func toSubmitWagerResponse(result *service.ProcessWagerResult) submitWagerResponse {
	resp := submitWagerResponse{
		TransactionID: result.TransactionID,
		Status:        string(result.Status),
		Direction:     string(result.Direction),
	}
	if result.Status == domain.StatusProcessed {
		balance := result.BalanceAfter
		resp.BalanceAfter = &balance
	}
	if result.FailureCode != nil {
		code := string(*result.FailureCode)
		resp.FailureCode = &code
	}
	return resp
}

type wagerTransactionResponse struct {
	ID                             string        `json:"id"`
	ProviderID                     string        `json:"providerId,omitempty"`
	PlayerID                       string        `json:"playerId"`
	WalletID                       string        `json:"walletId"`
	RoundID                        string        `json:"roundId,omitempty"`
	GameID                         string        `json:"gameId,omitempty"`
	Kind                           string        `json:"kind"`
	Status                         string        `json:"status"`
	Direction                      string        `json:"direction"`
	Amount                         domain.Money  `json:"amount"`
	BalanceAfter                   *domain.Money `json:"balanceAfter,omitempty"`
	ExternalTransactionID          string        `json:"externalTransactionId,omitempty"`
	ReferenceTransactionID         *string       `json:"referenceTransactionId,omitempty"`
	ReferenceExternalTransactionID *string       `json:"referenceExternalTransactionId,omitempty"`
	ReversedByTransactionID        *string       `json:"reversedByTransactionId,omitempty"`
	FailureCode                    *string       `json:"failureCode,omitempty"`
	CreatedAt                      time.Time     `json:"createdAt"`
	UpdatedAt                      time.Time     `json:"updatedAt"`
}

func toWagerTransactionResponse(tx *domain.WagerTransaction) wagerTransactionResponse {
	var failureCode *string
	if tx.FailureCode() != nil {
		code := string(*tx.FailureCode())
		failureCode = &code
	}
	return wagerTransactionResponse{
		ID:                             tx.ID(),
		ProviderID:                     tx.ProviderID(),
		PlayerID:                       tx.PlayerID(),
		WalletID:                       tx.WalletID(),
		RoundID:                        tx.RoundID(),
		GameID:                         tx.GameID(),
		Kind:                           string(tx.Kind()),
		Status:                         string(tx.Status()),
		Direction:                      string(tx.Direction()),
		Amount:                         tx.Amount(),
		BalanceAfter:                   tx.BalanceAfter(),
		ExternalTransactionID:          tx.ExternalTransactionID(),
		ReferenceTransactionID:         tx.ReferenceTransactionID(),
		ReferenceExternalTransactionID: tx.ReferenceExternalTransactionID(),
		ReversedByTransactionID:        tx.ReversedBy(),
		FailureCode:                    failureCode,
		CreatedAt:                      tx.CreatedAt(),
		UpdatedAt:                      tx.UpdatedAt(),
	}
}

func (h *Wagering) Get(w http.ResponseWriter, r *http.Request) {
	transactionID := r.PathValue("transactionId")
	identity, _ := middleware.IdentityFromContext(r.Context())

	tx, err := h.get.Execute(r.Context(), service.GetTransactionQuery{
		TransactionID:       transactionID,
		RequesterProviderID: identity.ProviderID,
	})
	if err != nil {
		writeServiceError(w, err, "WAGER_TRANSACTION_NOT_FOUND", "wager transaction not found")
		return
	}

	response.JSON(w, http.StatusOK, toWagerTransactionResponse(tx))
}

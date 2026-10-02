package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/response"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/application/service"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type Wallet struct {
	open      *service.OpenWalletService
	get       *service.GetWalletService
	list      *service.ListLedgerService
	reconcile *service.ReconcileWalletService
}

func NewWallet(open *service.OpenWalletService, get *service.GetWalletService, list *service.ListLedgerService, reconcile *service.ReconcileWalletService) *Wallet {
	return &Wallet{open: open, get: get, list: list, reconcile: reconcile}
}

type openWalletRequest struct {
	PlayerID       string `json:"playerId"`
	Currency       string `json:"currency"`
	OpeningBalance string `json:"openingBalance"`
}

type walletResponse struct {
	ID        string       `json:"id"`
	PlayerID  string       `json:"playerId"`
	Balance   domain.Money `json:"balance"`
	Version   int64        `json:"version"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

func toWalletResponse(wallet *domain.Wallet) walletResponse {
	return walletResponse{
		ID:        wallet.ID(),
		PlayerID:  wallet.PlayerID(),
		Balance:   wallet.Balance(),
		Version:   wallet.Version(),
		CreatedAt: wallet.CreatedAt(),
		UpdatedAt: wallet.UpdatedAt(),
	}
}

func (h *Wallet) Open(w http.ResponseWriter, r *http.Request) {
	var req openWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	wallet, err := h.open.Execute(r.Context(), service.OpenWalletCommand{
		PlayerID:       req.PlayerID,
		Currency:       req.Currency,
		OpeningBalance: req.OpeningBalance,
	})
	if err != nil {
		writeServiceError(w, err, "WALLET_NOT_FOUND", "wallet not found")
		return
	}

	response.JSON(w, http.StatusCreated, toWalletResponse(wallet))
}

func (h *Wallet) Get(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")

	wallet, err := h.get.Execute(r.Context(), walletID)
	if err != nil {
		writeServiceError(w, err, "WALLET_NOT_FOUND", "wallet not found")
		return
	}

	response.JSON(w, http.StatusOK, toWalletResponse(wallet))
}

func (h *Wallet) Ledger(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	page, err := h.list.Execute(r.Context(), service.ListLedgerQuery{
		WalletID: walletID,
		Cursor:   r.URL.Query().Get("cursor"),
		Limit:    limit,
	})
	if err != nil {
		writeServiceError(w, err, "WALLET_NOT_FOUND", "wallet not found")
		return
	}

	response.JSON(w, http.StatusOK, toLedgerPageResponse(page))
}

func (h *Wallet) Reconcile(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")

	result, err := h.reconcile.Execute(r.Context(), walletID)
	if err != nil {
		writeServiceError(w, err, "WALLET_NOT_FOUND", "wallet not found")
		return
	}

	response.JSON(w, http.StatusOK, toReconciliationResponse(result))
}

type reconciliationResponse struct {
	WalletID        string        `json:"walletId"`
	RecordedBalance domain.Money  `json:"recordedBalance"`
	ComputedBalance domain.Money  `json:"computedBalance"`
	Balanced        bool          `json:"balanced"`
	Divergence      *domain.Money `json:"divergence"`
}

func toReconciliationResponse(result *service.ReconciliationResult) reconciliationResponse {
	return reconciliationResponse{
		WalletID:        result.WalletID,
		RecordedBalance: result.RecordedBalance,
		ComputedBalance: result.ComputedBalance,
		Balanced:        result.Balanced,
		Divergence:      result.Divergence,
	}
}

type ledgerEntryResponse struct {
	ID            string       `json:"id"`
	WalletID      string       `json:"walletId"`
	TransactionID string       `json:"transactionId"`
	Direction     string       `json:"direction"`
	Amount        domain.Money `json:"amount"`
	BalanceBefore domain.Money `json:"balanceBefore"`
	BalanceAfter  domain.Money `json:"balanceAfter"`
	CreatedAt     time.Time    `json:"createdAt"`
}

type ledgerPageResponse struct {
	Entries    []ledgerEntryResponse `json:"entries"`
	NextCursor string                `json:"nextCursor"`
}

func toLedgerPageResponse(page *service.LedgerPage) ledgerPageResponse {
	entries := make([]ledgerEntryResponse, len(page.Entries))
	for i, entry := range page.Entries {
		entries[i] = ledgerEntryResponse{
			ID:            entry.ID(),
			WalletID:      entry.WalletID(),
			TransactionID: entry.TransactionID(),
			Direction:     string(entry.Direction()),
			Amount:        entry.Amount(),
			BalanceBefore: entry.BalanceBefore(),
			BalanceAfter:  entry.BalanceAfter(),
			CreatedAt:     entry.CreatedAt(),
		}
	}
	return ledgerPageResponse{Entries: entries, NextCursor: page.NextCursor}
}

func writeServiceError(w http.ResponseWriter, err error, notFoundCode, notFoundMessage string) {
	var domainErr *domain.Error
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(w, http.StatusNotFound, notFoundCode, notFoundMessage)
	case errors.Is(err, repository.ErrAlreadyExists):
		response.Error(w, http.StatusConflict, "WALLET_ALREADY_EXISTS", "wallet already exists for this player and currency")
	case errors.Is(err, service.ErrInvalidCursor):
		response.Error(w, http.StatusBadRequest, "INVALID_CURSOR", "invalid pagination cursor")
	case errors.Is(err, repository.ErrIdempotencyKeyConflict):
		response.Error(w, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "idempotency key already used with different content")
	case errors.Is(err, repository.ErrExternalTransactionConflict):
		response.Error(w, http.StatusConflict, "EXTERNAL_TRANSACTION_CONFLICT", "external transaction id already used with different content")
	case errors.As(err, &domainErr):
		response.Error(w, http.StatusBadRequest, string(domainErr.Code), domainErr.Message)
	default:
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected error")
	}
}

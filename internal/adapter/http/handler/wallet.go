package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/response"
	"github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/application/service"
	"github.com/rrenannn/junglegaming-challenge/internal/domain"
)

type Wallet struct {
	open *service.OpenWalletService
	get  *service.GetWalletService
}

func NewWallet(open *service.OpenWalletService, get *service.GetWalletService) *Wallet {
	return &Wallet{open: open, get: get}
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
		writeWalletError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, toWalletResponse(wallet))
}

func (h *Wallet) Get(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")

	wallet, err := h.get.Execute(r.Context(), walletID)
	if err != nil {
		writeWalletError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, toWalletResponse(wallet))
}

func writeWalletError(w http.ResponseWriter, err error) {
	var domainErr *domain.Error
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(w, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
	case errors.Is(err, repository.ErrAlreadyExists):
		response.Error(w, http.StatusConflict, "WALLET_ALREADY_EXISTS", "wallet already exists for this player and currency")
	case errors.As(err, &domainErr):
		response.Error(w, http.StatusBadRequest, string(domainErr.Code), domainErr.Message)
	default:
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected error")
	}
}

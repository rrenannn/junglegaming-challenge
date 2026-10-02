package domain

import "errors"

type FailureCode string

const (
	FailureInsufficientFunds            FailureCode = "INSUFFICIENT_FUNDS"
	FailureInsufficientFundsForReversal FailureCode = "INSUFFICIENT_FUNDS_FOR_REVERSAL"
	FailureReferenceNotFound            FailureCode = "REFERENCE_NOT_FOUND"
	FailureReferenceNotProcessed        FailureCode = "REFERENCE_NOT_PROCESSED"
	FailureReferenceRejected            FailureCode = "REFERENCE_REJECTED"
	FailureReferenceMismatch            FailureCode = "REFERENCE_MISMATCH"
	FailureAlreadyReversed              FailureCode = "ALREADY_REVERSED"
	FailureCurrencyMismatch             FailureCode = "CURRENCY_MISMATCH"
	FailureInvalidAmount                FailureCode = "INVALID_AMOUNT"
	FailureInvalidTransactionKind       FailureCode = "INVALID_TRANSACTION_KIND"
	FailureInvalidCurrency              FailureCode = "INVALID_CURRENCY"
	FailureInvalidState                 FailureCode = "INVALID_STATE"
	FailureAmountOverflow               FailureCode = "AMOUNT_OVERFLOW"
)

type Error struct {
	Code    FailureCode
	Message string
}

func NewError(code FailureCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

func (e *Error) Error() string {
	return string(e.Code) + ": " + e.Message
}

func HasFailureCode(err error, code FailureCode) bool {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Code == code
	}
	return false
}

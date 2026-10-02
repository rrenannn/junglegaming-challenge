package repository

import "errors"

var ErrNotFound = errors.New("record not found")
var ErrAlreadyExists = errors.New("record already exists")
var ErrIdempotencyKeyConflict = errors.New("idempotency key already used with different content")
var ErrExternalTransactionConflict = errors.New("external transaction id already used with different content")

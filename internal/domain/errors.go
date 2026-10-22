package domain

import "errors"

var (
	ErrAccountNotFound           = errors.New("account not found")
	ErrInsufficientAvailableFunds = errors.New("insufficient available funds to approve authorization")
	ErrAuthorizationNotFound     = errors.New("authorization hold not found")
	ErrAuthorizationAlreadyClosed = errors.New("authorization already settled or expired")
	ErrInvalidValueDate          = errors.New("value date must be positive and valid")
	ErrTransactionNotFound       = errors.New("transaction not found for reversal")
	ErrTransactionAlreadyReversed= errors.New("transaction has already been reversed")
	ErrNegativeCredit            = errors.New("credit amount must be strictly positive")
	ErrNegativeDebit             = errors.New("debit amount must be strictly positive")
)

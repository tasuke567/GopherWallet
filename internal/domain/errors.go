package domain

import "errors"

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrSameAccount         = errors.New("cannot transfer to the same account")
	ErrInvalidAmount       = errors.New("transfer amount must be positive")
	ErrCurrencyMismatch    = errors.New("currency mismatch between accounts")
	ErrDuplicateTransaction = errors.New("duplicate transaction (idempotency key already used)")
)

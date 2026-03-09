package domain

import (
	"context"
	"time"
)

type TransactionStatus string

const (
	TransactionStatusPending TransactionStatus = "pending"
	TransactionStatusSuccess TransactionStatus = "success"
	TransactionStatusFailed  TransactionStatus = "failed"
)

type TransferTransaction struct {
	ID            int64             `json:"id" db:"id"`
	FromAccountID int64             `json:"from_account_id" db:"from_account_id"`
	ToAccountID   int64             `json:"to_account_id" db:"to_account_id"`
	Amount        int64             `json:"amount" db:"amount"`
	Currency      string            `json:"currency" db:"currency"`
	Status        TransactionStatus `json:"status" db:"status"`
	IdempotencyKey string           `json:"idempotency_key" db:"idempotency_key"`
	CreatedAt     time.Time         `json:"created_at" db:"created_at"`
}

type TransactionRepository interface {
	Create(ctx context.Context, tx Transaction, txn *TransferTransaction) (*TransferTransaction, error)
	UpdateStatus(ctx context.Context, tx Transaction, id int64, status TransactionStatus) error
	GetByIdempotencyKey(ctx context.Context, key string) (*TransferTransaction, error)
}

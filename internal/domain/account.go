package domain

import (
	"context"
	"time"
)

type Account struct {
	ID        int64     `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Balance   int64     `json:"balance" db:"balance"` // stored in smallest unit (satang/cents)
	Currency  string    `json:"currency" db:"currency"`
	Version   int       `json:"version" db:"version"` // optimistic locking
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type AccountRepository interface {
	Create(ctx context.Context, account *Account) error
	GetByID(ctx context.Context, id int64) (*Account, error)
	GetByUserID(ctx context.Context, userID string) ([]Account, error)

	// GetByIDForUpdate acquires a row-level lock (SELECT ... FOR UPDATE)
	// to prevent concurrent modifications during a transaction.
	GetByIDForUpdate(ctx context.Context, tx Transaction, id int64) (*Account, error)

	// UpdateBalance updates balance within an existing transaction.
	UpdateBalance(ctx context.Context, tx Transaction, id int64, amount int64, newVersion int) error
}

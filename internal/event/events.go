package event

import "time"

// TransferCompleted is published to the message broker after a successful transfer.
type TransferCompleted struct {
	TransactionID int64     `json:"transaction_id"`
	FromAccountID int64     `json:"from_account_id"`
	ToAccountID   int64     `json:"to_account_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Timestamp     time.Time `json:"timestamp"`
}

const SubjectTransferCompleted = "wallet.transfer.completed"

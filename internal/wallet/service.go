package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/wey/gopher-wallet/internal/domain"
	"github.com/wey/gopher-wallet/internal/event"
)

type TransferRequest struct {
	FromAccountID  int64  `json:"from_account_id" validate:"required"`
	ToAccountID    int64  `json:"to_account_id" validate:"required"`
	Amount         int64  `json:"amount" validate:"required,gt=0"`
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

type TransferService struct {
	accountRepo domain.AccountRepository
	txnRepo     domain.TransactionRepository
	txManager   domain.TransactionManager
	publisher   event.Publisher // nil = events disabled
	logger      *slog.Logger
}

func NewTransferService(
	accountRepo domain.AccountRepository,
	txnRepo domain.TransactionRepository,
	txManager domain.TransactionManager,
	logger *slog.Logger,
) *TransferService {
	return &TransferService{
		accountRepo: accountRepo,
		txnRepo:     txnRepo,
		txManager:   txManager,
		logger:      logger,
	}
}

// WithPublisher sets the event publisher for async notifications.
func (s *TransferService) WithPublisher(p event.Publisher) *TransferService {
	s.publisher = p
	return s
}

// Transfer executes a money transfer between two accounts inside a single
// database transaction. It uses SELECT ... FOR UPDATE to lock both rows,
// preventing race conditions when 1,000+ users transfer concurrently.
//
// Flow:
//  1. Check idempotency key (prevent duplicate transfers)
//  2. BEGIN transaction
//  3. Lock sender account (SELECT ... FOR UPDATE) — ordered by ID to prevent deadlocks
//  4. Lock receiver account (SELECT ... FOR UPDATE)
//  5. Validate balance and currency
//  6. Debit sender, credit receiver
//  7. Create transaction record
//  8. COMMIT (or ROLLBACK on any error)
func (s *TransferService) Transfer(ctx context.Context, req TransferRequest) (*domain.TransferTransaction, error) {
	// --- Validation ---
	if req.FromAccountID == req.ToAccountID {
		return nil, domain.ErrSameAccount
	}
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	// --- Idempotency check: return existing result if key was used before ---
	existing, err := s.txnRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("check idempotency: %w", err)
	}
	if existing != nil {
		s.logger.Info("duplicate transfer detected", "idempotency_key", req.IdempotencyKey)
		return existing, domain.ErrDuplicateTransaction
	}

	var result *domain.TransferTransaction

	// --- Execute everything inside a single database transaction ---
	err = s.txManager.WithTransaction(ctx, func(tx domain.Transaction) error {
		// Lock accounts in a consistent order (smaller ID first) to prevent deadlocks.
		// This is a critical pattern for high-concurrency systems.
		firstID, secondID := req.FromAccountID, req.ToAccountID
		if firstID > secondID {
			firstID, secondID = secondID, firstID
		}

		first, err := s.accountRepo.GetByIDForUpdate(ctx, tx, firstID)
		if err != nil {
			return fmt.Errorf("lock account %d: %w", firstID, err)
		}

		second, err := s.accountRepo.GetByIDForUpdate(ctx, tx, secondID)
		if err != nil {
			return fmt.Errorf("lock account %d: %w", secondID, err)
		}

		// Map back to sender/receiver
		var sender, receiver *domain.Account
		if first.ID == req.FromAccountID {
			sender, receiver = first, second
		} else {
			sender, receiver = second, first
		}

		// --- Currency validation ---
		if sender.Currency != receiver.Currency {
			return domain.ErrCurrencyMismatch
		}

		// --- Balance check ---
		if sender.Balance < req.Amount {
			return domain.ErrInsufficientBalance
		}

		// --- Create the transaction record (status: pending) ---
		txn := &domain.TransferTransaction{
			FromAccountID:  req.FromAccountID,
			ToAccountID:    req.ToAccountID,
			Amount:         req.Amount,
			Currency:       sender.Currency,
			Status:         domain.TransactionStatusPending,
			IdempotencyKey: req.IdempotencyKey,
		}

		txn, err = s.txnRepo.Create(ctx, tx, txn)
		if err != nil {
			return fmt.Errorf("create transaction record: %w", err)
		}

		// --- Debit sender ---
		if err := s.accountRepo.UpdateBalance(ctx, tx, sender.ID, -req.Amount, sender.Version+1); err != nil {
			return fmt.Errorf("debit sender: %w", err)
		}

		// --- Credit receiver ---
		if err := s.accountRepo.UpdateBalance(ctx, tx, receiver.ID, req.Amount, receiver.Version+1); err != nil {
			return fmt.Errorf("credit receiver: %w", err)
		}

		// --- Mark transaction as success ---
		if err := s.txnRepo.UpdateStatus(ctx, tx, txn.ID, domain.TransactionStatusSuccess); err != nil {
			return fmt.Errorf("update transaction status: %w", err)
		}

		txn.Status = domain.TransactionStatusSuccess
		result = txn
		return nil
	})

	if err != nil {
		s.logger.Error("transfer failed",
			"from", req.FromAccountID,
			"to", req.ToAccountID,
			"amount", req.Amount,
			"error", err,
		)
		return nil, err
	}

	s.logger.Info("transfer completed",
		"transaction_id", result.ID,
		"from", req.FromAccountID,
		"to", req.ToAccountID,
		"amount", req.Amount,
	)

	// --- Publish event asynchronously (fire-and-forget) ---
	if s.publisher != nil {
		go s.publishTransferEvent(ctx, result)
	}

	return result, nil
}

func (s *TransferService) publishTransferEvent(ctx context.Context, txn *domain.TransferTransaction) {
	evt := event.TransferCompleted{
		TransactionID: txn.ID,
		FromAccountID: txn.FromAccountID,
		ToAccountID:   txn.ToAccountID,
		Amount:        txn.Amount,
		Currency:      txn.Currency,
		Timestamp:     time.Now(),
	}

	data, err := json.Marshal(evt)
	if err != nil {
		s.logger.Error("failed to marshal transfer event", "error", err)
		return
	}

	if err := s.publisher.Publish(ctx, event.SubjectTransferCompleted, data); err != nil {
		s.logger.Error("failed to publish transfer event", "error", err)
	}
}

package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wey/gopher-wallet/internal/domain"
)

type pgTx struct {
	tx pgx.Tx
}

type PgTransactionManager struct {
	pool *pgxpool.Pool
}

func NewPgTransactionManager(pool *pgxpool.Pool) *PgTransactionManager {
	return &PgTransactionManager{pool: pool}
}

func (m *PgTransactionManager) WithTransaction(ctx context.Context, fn func(tx domain.Transaction) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(&pgTx{tx: tx}); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func extractTx(tx domain.Transaction) (pgx.Tx, error) {
	ptx, ok := tx.(*pgTx)
	if !ok {
		return nil, fmt.Errorf("invalid transaction type")
	}
	return ptx.tx, nil
}

// --- Account Repository ---

type AccountRepo struct {
	pool *pgxpool.Pool
}

func NewAccountRepo(pool *pgxpool.Pool) *AccountRepo {
	return &AccountRepo{pool: pool}
}

func (r *AccountRepo) Create(ctx context.Context, account *domain.Account) error {
	query := `
		INSERT INTO accounts (user_id, balance, currency)
		VALUES ($1, $2, $3)
		RETURNING id, version, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		account.UserID, account.Balance, account.Currency,
	).Scan(&account.ID, &account.Version, &account.CreatedAt, &account.UpdatedAt)
}

func (r *AccountRepo) GetByID(ctx context.Context, id int64) (*domain.Account, error) {
	query := `SELECT id, user_id, balance, currency, version, created_at, updated_at
	           FROM accounts WHERE id = $1`

	var a domain.Account
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.Balance, &a.Currency,
		&a.Version, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account by id: %w", err)
	}
	return &a, nil
}

func (r *AccountRepo) GetByUserID(ctx context.Context, userID string) ([]domain.Account, error) {
	query := `SELECT id, user_id, balance, currency, version, created_at, updated_at
	           FROM accounts WHERE user_id = $1`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get accounts by user_id: %w", err)
	}
	defer rows.Close()

	var accounts []domain.Account
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.UserID, &a.Balance, &a.Currency,
			&a.Version, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// GetByIDForUpdate uses SELECT ... FOR UPDATE to acquire a row-level lock.
// This prevents other transactions from modifying the row until this transaction commits.
func (r *AccountRepo) GetByIDForUpdate(ctx context.Context, tx domain.Transaction, id int64) (*domain.Account, error) {
	pgTx, err := extractTx(tx)
	if err != nil {
		return nil, err
	}

	query := `SELECT id, user_id, balance, currency, version, created_at, updated_at
	           FROM accounts WHERE id = $1 FOR UPDATE`

	var a domain.Account
	err = pgTx.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.Balance, &a.Currency,
		&a.Version, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account for update: %w", err)
	}
	return &a, nil
}

func (r *AccountRepo) UpdateBalance(ctx context.Context, tx domain.Transaction, id int64, amount int64, newVersion int) error {
	pgTx, err := extractTx(tx)
	if err != nil {
		return err
	}

	query := `UPDATE accounts SET balance = balance + $1, version = $2, updated_at = NOW()
	           WHERE id = $3`

	_, err = pgTx.Exec(ctx, query, amount, newVersion, id)
	if err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	return nil
}

// --- Transaction Repository ---

type TransactionRepo struct {
	pool *pgxpool.Pool
}

func NewTransactionRepo(pool *pgxpool.Pool) *TransactionRepo {
	return &TransactionRepo{pool: pool}
}

func (r *TransactionRepo) Create(ctx context.Context, tx domain.Transaction, txn *domain.TransferTransaction) (*domain.TransferTransaction, error) {
	pgTx, err := extractTx(tx)
	if err != nil {
		return nil, err
	}

	query := `INSERT INTO transactions (from_account_id, to_account_id, amount, currency, status, idempotency_key)
	           VALUES ($1, $2, $3, $4, $5, $6)
	           RETURNING id, created_at`

	err = pgTx.QueryRow(ctx, query,
		txn.FromAccountID, txn.ToAccountID, txn.Amount,
		txn.Currency, txn.Status, txn.IdempotencyKey,
	).Scan(&txn.ID, &txn.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}
	return txn, nil
}

func (r *TransactionRepo) UpdateStatus(ctx context.Context, tx domain.Transaction, id int64, status domain.TransactionStatus) error {
	pgTx, err := extractTx(tx)
	if err != nil {
		return err
	}

	query := `UPDATE transactions SET status = $1 WHERE id = $2`
	_, err = pgTx.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update transaction status: %w", err)
	}
	return nil
}

func (r *TransactionRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.TransferTransaction, error) {
	query := `SELECT id, from_account_id, to_account_id, amount, currency, status, idempotency_key, created_at
	           FROM transactions WHERE idempotency_key = $1`

	var t domain.TransferTransaction
	err := r.pool.QueryRow(ctx, query, key).Scan(
		&t.ID, &t.FromAccountID, &t.ToAccountID, &t.Amount,
		&t.Currency, &t.Status, &t.IdempotencyKey, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get transaction by idempotency key: %w", err)
	}
	return &t, nil
}

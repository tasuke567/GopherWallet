package wallet_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wey/gopher-wallet/internal/domain"
	"github.com/wey/gopher-wallet/internal/wallet"
)

// --- Mock Transaction ---

type mockTx struct{}

// --- Mock Transaction Manager ---

type mockTxManager struct{}

func (m *mockTxManager) WithTransaction(ctx context.Context, fn func(tx domain.Transaction) error) error {
	return fn(&mockTx{})
}

// --- Mock Account Repository ---

type mockAccountRepo struct {
	mu       sync.Mutex
	accounts map[int64]*domain.Account
}

func newMockAccountRepo() *mockAccountRepo {
	return &mockAccountRepo{accounts: make(map[int64]*domain.Account)}
}

func (r *mockAccountRepo) Create(ctx context.Context, account *domain.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account.ID = int64(len(r.accounts) + 1)
	account.Version = 1
	stored := *account
	r.accounts[account.ID] = &stored
	return nil
}

func (r *mockAccountRepo) GetByID(ctx context.Context, id int64) (*domain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return nil, domain.ErrAccountNotFound
	}
	copy := *a
	return &copy, nil
}

func (r *mockAccountRepo) GetByUserID(ctx context.Context, userID string) ([]domain.Account, error) {
	return nil, nil
}

func (r *mockAccountRepo) GetByIDForUpdate(ctx context.Context, tx domain.Transaction, id int64) (*domain.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return nil, domain.ErrAccountNotFound
	}
	copy := *a
	return &copy, nil
}

func (r *mockAccountRepo) UpdateBalance(ctx context.Context, tx domain.Transaction, id int64, amount int64, newVersion int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return domain.ErrAccountNotFound
	}
	a.Balance += amount
	a.Version = newVersion
	return nil
}

// --- Mock Transaction Repository ---

type mockTxnRepo struct {
	mu           sync.Mutex
	transactions map[string]*domain.TransferTransaction
	nextID       int64
}

func newMockTxnRepo() *mockTxnRepo {
	return &mockTxnRepo{transactions: make(map[string]*domain.TransferTransaction), nextID: 1}
}

func (r *mockTxnRepo) Create(ctx context.Context, tx domain.Transaction, txn *domain.TransferTransaction) (*domain.TransferTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	txn.ID = r.nextID
	r.nextID++
	stored := *txn
	r.transactions[txn.IdempotencyKey] = &stored
	return txn, nil
}

func (r *mockTxnRepo) UpdateStatus(ctx context.Context, tx domain.Transaction, id int64, status domain.TransactionStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.transactions {
		if t.ID == id {
			t.Status = status
			return nil
		}
	}
	return nil
}

func (r *mockTxnRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.TransferTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.transactions[key]
	if !ok {
		return nil, nil
	}
	copy := *t
	return &copy, nil
}

// --- Tests ---

func newTestService() (*wallet.TransferService, *mockAccountRepo) {
	accountRepo := newMockAccountRepo()
	txnRepo := newMockTxnRepo()
	txManager := &mockTxManager{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Seed two accounts
	accountRepo.accounts[1] = &domain.Account{ID: 1, UserID: "user-a", Balance: 100000, Currency: "THB", Version: 1}
	accountRepo.accounts[2] = &domain.Account{ID: 2, UserID: "user-b", Balance: 50000, Currency: "THB", Version: 1}

	svc := wallet.NewTransferService(accountRepo, txnRepo, txManager, logger)
	return svc, accountRepo
}

func TestTransfer_Success(t *testing.T) {
	svc, accountRepo := newTestService()

	result, err := svc.Transfer(context.Background(), wallet.TransferRequest{
		FromAccountID:  1,
		ToAccountID:    2,
		Amount:         10000, // 100.00 THB
		IdempotencyKey: "txn-001",
	})

	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusSuccess, result.Status)
	assert.Equal(t, int64(10000), result.Amount)

	// Verify balances
	sender, _ := accountRepo.GetByID(context.Background(), 1)
	receiver, _ := accountRepo.GetByID(context.Background(), 2)
	assert.Equal(t, int64(90000), sender.Balance)
	assert.Equal(t, int64(60000), receiver.Balance)
}

func TestTransfer_InsufficientBalance(t *testing.T) {
	svc, _ := newTestService()

	_, err := svc.Transfer(context.Background(), wallet.TransferRequest{
		FromAccountID:  1,
		ToAccountID:    2,
		Amount:         999999, // way more than balance
		IdempotencyKey: "txn-002",
	})

	assert.ErrorIs(t, err, domain.ErrInsufficientBalance)
}

func TestTransfer_SameAccount(t *testing.T) {
	svc, _ := newTestService()

	_, err := svc.Transfer(context.Background(), wallet.TransferRequest{
		FromAccountID:  1,
		ToAccountID:    1,
		Amount:         10000,
		IdempotencyKey: "txn-003",
	})

	assert.ErrorIs(t, err, domain.ErrSameAccount)
}

func TestTransfer_InvalidAmount(t *testing.T) {
	svc, _ := newTestService()

	_, err := svc.Transfer(context.Background(), wallet.TransferRequest{
		FromAccountID:  1,
		ToAccountID:    2,
		Amount:         -100,
		IdempotencyKey: "txn-004",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidAmount)
}

func TestTransfer_DuplicateIdempotencyKey(t *testing.T) {
	svc, _ := newTestService()

	// First transfer succeeds
	_, err := svc.Transfer(context.Background(), wallet.TransferRequest{
		FromAccountID:  1,
		ToAccountID:    2,
		Amount:         10000,
		IdempotencyKey: "txn-dup",
	})
	require.NoError(t, err)

	// Second transfer with same key returns duplicate error
	_, err = svc.Transfer(context.Background(), wallet.TransferRequest{
		FromAccountID:  1,
		ToAccountID:    2,
		Amount:         10000,
		IdempotencyKey: "txn-dup",
	})
	assert.ErrorIs(t, err, domain.ErrDuplicateTransaction)
}

func TestTransfer_AccountNotFound(t *testing.T) {
	svc, _ := newTestService()

	_, err := svc.Transfer(context.Background(), wallet.TransferRequest{
		FromAccountID:  1,
		ToAccountID:    999, // doesn't exist
		Amount:         10000,
		IdempotencyKey: "txn-005",
	})

	assert.Error(t, err)
}

func TestTransfer_ConcurrentTransfers(t *testing.T) {
	svc, accountRepo := newTestService()
	// Give account 1 enough balance for 10 transfers of 5000
	accountRepo.mu.Lock()
	accountRepo.accounts[1].Balance = 50000
	accountRepo.mu.Unlock()

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Transfer(context.Background(), wallet.TransferRequest{
				FromAccountID:  1,
				ToAccountID:    2,
				Amount:         5000,
				IdempotencyKey: "concurrent-" + string(rune('0'+i)),
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// With mock (no real locking), some may succeed and some may fail
	// but data should not become negative in the real implementation
	sender, _ := accountRepo.GetByID(context.Background(), 1)
	t.Logf("Sender final balance: %d", sender.Balance)
	t.Logf("Errors: %d", len(errCh))
}

// --- Benchmarks ---

func BenchmarkTransfer(b *testing.B) {
	accountRepo := newMockAccountRepo()
	txnRepo := newMockTxnRepo()
	txManager := &mockTxManager{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	accountRepo.accounts[1] = &domain.Account{ID: 1, UserID: "user-a", Balance: math.MaxInt64, Currency: "THB", Version: 1}
	accountRepo.accounts[2] = &domain.Account{ID: 2, UserID: "user-b", Balance: 0, Currency: "THB", Version: 1}

	svc := wallet.NewTransferService(accountRepo, txnRepo, txManager, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Transfer(context.Background(), wallet.TransferRequest{
			FromAccountID:  1,
			ToAccountID:    2,
			Amount:         100,
			IdempotencyKey: fmt.Sprintf("bench-%d", i),
		})
	}
}

func BenchmarkTransfer_Parallel(b *testing.B) {
	accountRepo := newMockAccountRepo()
	txnRepo := newMockTxnRepo()
	txManager := &mockTxManager{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	accountRepo.accounts[1] = &domain.Account{ID: 1, UserID: "user-a", Balance: math.MaxInt64, Currency: "THB", Version: 1}
	accountRepo.accounts[2] = &domain.Account{ID: 2, UserID: "user-b", Balance: 0, Currency: "THB", Version: 1}

	svc := wallet.NewTransferService(accountRepo, txnRepo, txManager, logger)

	var counter atomic.Int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := counter.Add(1)
			_, _ = svc.Transfer(context.Background(), wallet.TransferRequest{
				FromAccountID:  1,
				ToAccountID:    2,
				Amount:         100,
				IdempotencyKey: fmt.Sprintf("bench-p-%d", id),
			})
		}
	})
}

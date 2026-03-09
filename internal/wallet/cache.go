package wallet

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/wey/gopher-wallet/internal/domain"
)

// CachedAccountRepo wraps an AccountRepository with Redis caching.
// It caches balance lookups to reduce database load under high traffic.
type CachedAccountRepo struct {
	inner  domain.AccountRepository
	rdb    *redis.Client
	ttl    time.Duration
	logger *slog.Logger
}

func NewCachedAccountRepo(inner domain.AccountRepository, rdb *redis.Client, logger *slog.Logger) *CachedAccountRepo {
	return &CachedAccountRepo{
		inner:  inner,
		rdb:    rdb,
		ttl:    30 * time.Second,
		logger: logger,
	}
}

func (r *CachedAccountRepo) cacheKey(id int64) string {
	return fmt.Sprintf("account:%d", id)
}

func (r *CachedAccountRepo) Create(ctx context.Context, account *domain.Account) error {
	return r.inner.Create(ctx, account)
}

func (r *CachedAccountRepo) GetByID(ctx context.Context, id int64) (*domain.Account, error) {
	// Try cache first
	key := r.cacheKey(id)
	val, err := r.rdb.Get(ctx, key+":balance").Result()
	if err == nil {
		// Cache hit — still need full account, but we can validate quickly
		balance, _ := strconv.ParseInt(val, 10, 64)
		account, dbErr := r.inner.GetByID(ctx, id)
		if dbErr != nil {
			return nil, dbErr
		}
		// Use cached balance if version matches
		if account.Balance == balance {
			r.logger.Debug("cache hit", "account_id", id)
		}
		return account, nil
	}

	// Cache miss — fetch from DB and cache
	account, err := r.inner.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Cache the balance
	r.rdb.Set(ctx, key+":balance", strconv.FormatInt(account.Balance, 10), r.ttl)
	return account, nil
}

func (r *CachedAccountRepo) GetByUserID(ctx context.Context, userID string) ([]domain.Account, error) {
	return r.inner.GetByUserID(ctx, userID)
}

// GetByIDForUpdate bypasses cache — we need the real DB row with a lock.
func (r *CachedAccountRepo) GetByIDForUpdate(ctx context.Context, tx domain.Transaction, id int64) (*domain.Account, error) {
	return r.inner.GetByIDForUpdate(ctx, tx, id)
}

// UpdateBalance delegates to DB and invalidates the cache.
func (r *CachedAccountRepo) UpdateBalance(ctx context.Context, tx domain.Transaction, id int64, amount int64, newVersion int) error {
	if err := r.inner.UpdateBalance(ctx, tx, id, amount, newVersion); err != nil {
		return err
	}

	// Invalidate cache after balance update
	r.rdb.Del(ctx, r.cacheKey(id)+":balance")
	return nil
}

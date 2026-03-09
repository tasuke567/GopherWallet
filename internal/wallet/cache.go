package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/wey/gopher-wallet/internal/domain"
	"github.com/wey/gopher-wallet/internal/middleware"
	"github.com/wey/gopher-wallet/internal/resilience"
)

// CachedAccountRepo wraps an AccountRepository with Redis caching and
// a circuit breaker. When Redis is down, the circuit opens and requests
// fall through directly to the database, preventing cascade failures.
type CachedAccountRepo struct {
	inner  domain.AccountRepository
	rdb    *redis.Client
	cb     *resilience.CircuitBreaker
	ttl    time.Duration
	logger *slog.Logger
}

func NewCachedAccountRepo(inner domain.AccountRepository, rdb *redis.Client, cb *resilience.CircuitBreaker, logger *slog.Logger) *CachedAccountRepo {
	return &CachedAccountRepo{
		inner:  inner,
		rdb:    rdb,
		cb:     cb,
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
	key := r.cacheKey(id)

	// Try cache first (protected by circuit breaker)
	var account domain.Account
	err := r.cb.Execute(func() error {
		val, err := r.rdb.Get(ctx, key).Result()
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(val), &account)
	})
	if err == nil {
		middleware.CacheHitsTotal.Inc()
		r.logger.Debug("cache hit", "account_id", id)
		return &account, nil
	}

	middleware.CacheMissesTotal.Inc()

	// Cache miss or circuit open — fetch from database
	acc, err := r.inner.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Cache the result (best effort)
	if data, marshalErr := json.Marshal(acc); marshalErr == nil {
		_ = r.cb.Execute(func() error {
			return r.rdb.Set(ctx, key, data, r.ttl).Err()
		})
	}

	return acc, nil
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
	_ = r.cb.Execute(func() error {
		return r.rdb.Del(ctx, r.cacheKey(id)).Err()
	})
	return nil
}

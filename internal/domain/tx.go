package domain

import "context"

// Transaction abstracts a database transaction so the domain layer
// is not coupled to any specific database driver.
type Transaction interface{}

// TransactionManager handles Begin/Commit/Rollback lifecycle.
type TransactionManager interface {
	// WithTransaction executes fn inside a database transaction.
	// It commits on success and rolls back on error.
	WithTransaction(ctx context.Context, fn func(tx Transaction) error) error
}

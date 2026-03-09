package resilience

import (
	"context"
	"math"
	"time"
)

// Retry executes fn up to maxAttempts times with exponential backoff.
// It respects context cancellation between attempts.
func Retry(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < maxAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}

		if i < maxAttempts-1 {
			delay := time.Duration(math.Pow(2, float64(i))) * baseDelay
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return err
}

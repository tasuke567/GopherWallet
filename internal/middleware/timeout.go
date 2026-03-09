package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
)

// TimeoutMiddleware sets a context deadline on each request to prevent
// long-running operations from blocking resources indefinitely.
func TimeoutMiddleware(timeout time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), timeout)
		defer cancel()
		c.SetUserContext(ctx)
		return c.Next()
	}
}

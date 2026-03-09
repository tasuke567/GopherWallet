package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// Idempotency middleware uses Redis to prevent duplicate API calls.
func Idempotency(rdb *redis.Client, ttl time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() != fiber.MethodPost && c.Method() != fiber.MethodPut {
			return c.Next()
		}

		key := c.Get("Idempotency-Key")
		if key == "" {
			return c.Next()
		}

		redisKey := "idempotency:" + key
		ctx := context.Background()

		ok, err := rdb.SetNX(ctx, redisKey, "processing", ttl).Result()
		if err != nil {
			// If Redis is down, let the request through (fail open).
			return c.Next()
		}

		if !ok {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "duplicate request: idempotency key already used",
			})
		}

		return c.Next()
	}
}

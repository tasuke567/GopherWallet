package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/wey/gopher-wallet/internal/middleware"
	"github.com/wey/gopher-wallet/internal/notification"
	"github.com/wey/gopher-wallet/internal/resilience"
	"github.com/wey/gopher-wallet/internal/wallet"
	"github.com/wey/gopher-wallet/pkg/config"
	"github.com/wey/gopher-wallet/pkg/database"
	"github.com/wey/gopher-wallet/pkg/messaging"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- Database (configurable pool) ---
	pool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	log.Info("connected to PostgreSQL",
		"max_conns", cfg.DBMaxConns,
		"min_conns", cfg.DBMinConns,
	)

	// Register DB pool metrics for Prometheus
	middleware.RegisterDBPoolMetrics(pool)

	// --- Redis (with pool sizing) ---
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisURL,
		PoolSize: cfg.RedisPoolSize,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Warn("redis not available, caching and idempotency will be degraded", "error", err)
	} else {
		log.Info("connected to Redis", "pool_size", cfg.RedisPoolSize)
	}
	defer rdb.Close()

	// --- Circuit Breaker for Redis ---
	cb := resilience.NewCircuitBreaker(
		cfg.CBMaxFailures,
		time.Duration(cfg.CBTimeoutSec)*time.Second,
	)

	// --- Repositories ---
	accountRepo := wallet.NewAccountRepo(pool)
	cachedRepo := wallet.NewCachedAccountRepo(accountRepo, rdb, cb, log)
	txnRepo := wallet.NewTransactionRepo(pool)
	txManager := wallet.NewPgTransactionManager(pool)

	transferSvc := wallet.NewTransferService(cachedRepo, txnRepo, txManager, log)

	// --- NATS (Event-Driven) ---
	var notifWorker *notification.Worker
	natsClient, err := messaging.NewNATSClient(cfg.NatsURL, log)
	if err != nil {
		log.Warn("NATS not available, events will be disabled", "error", err)
	} else {
		defer natsClient.Close()
		transferSvc.WithPublisher(natsClient)

		// Start notification worker pool
		notifWorker = notification.NewWorker(natsClient, log, cfg.WorkerPoolSize)
		if err := notifWorker.Start(); err != nil {
			log.Error("failed to start notification worker", "error", err)
		} else {
			log.Info("notification worker pool started", "pool_size", cfg.WorkerPoolSize)
		}
	}

	app := fiber.New(fiber.Config{
		AppName:      "GopherWallet v2.0",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	})

	// Middleware stack (order matters)
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New())
	app.Use(compress.New())
	app.Use(middleware.PrometheusMiddleware())
	app.Use(middleware.TimeoutMiddleware(time.Duration(cfg.RequestTimeoutSec) * time.Second))
	app.Use(limiter.New(limiter.Config{
		Max:        cfg.RateLimit,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded, try again later",
			})
		},
	}))
	app.Use(middleware.Idempotency(rdb, 24*time.Hour))

	// --- Health Endpoints ---
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "gopher-wallet"})
	})

	app.Get("/health/ready", func(c *fiber.Ctx) error {
		hctx := c.UserContext()
		checks := fiber.Map{}
		healthy := true

		// Database
		if err := pool.Ping(hctx); err != nil {
			checks["database"] = fiber.Map{"status": "unhealthy", "error": err.Error()}
			healthy = false
		} else {
			stat := pool.Stat()
			checks["database"] = fiber.Map{
				"status":         "healthy",
				"total_conns":    stat.TotalConns(),
				"idle_conns":     stat.IdleConns(),
				"acquired_conns": stat.AcquiredConns(),
			}
		}

		// Redis
		if err := rdb.Ping(hctx).Err(); err != nil {
			checks["redis"] = fiber.Map{"status": "degraded", "error": err.Error()}
		} else {
			checks["redis"] = fiber.Map{"status": "healthy"}
		}

		// Circuit Breaker
		checks["circuit_breaker"] = fiber.Map{"state": cb.State().String()}

		status := fiber.StatusOK
		statusText := "ready"
		if !healthy {
			status = fiber.StatusServiceUnavailable
			statusText = "not ready"
		}

		return c.Status(status).JSON(fiber.Map{
			"status": statusText,
			"checks": checks,
		})
	})

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	handler := wallet.NewHandler(transferSvc, cachedRepo, log)
	handler.RegisterRoutes(app)

	go func() {
		if err := app.Listen(cfg.ServerAddr()); err != nil {
			log.Error("server error", "error", err)
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	// 1. Stop accepting new requests
	if err := app.Shutdown(); err != nil {
		log.Error("server shutdown error", "error", err)
	}

	// 2. Drain notification worker queue
	if notifWorker != nil {
		notifWorker.Stop()
		log.Info("notification worker pool stopped")
	}

	// Defers handle: natsClient.Close(), rdb.Close(), pool.Close()
	log.Info("server stopped gracefully")
}

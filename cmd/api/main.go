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
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/wey/gopher-wallet/internal/middleware"
	"github.com/wey/gopher-wallet/internal/notification"
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

	pool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	log.Info("connected to PostgreSQL")

	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Warn("redis not available, idempotency middleware will be bypassed", "error", err)
	} else {
		log.Info("connected to Redis")
	}

	accountRepo := wallet.NewAccountRepo(pool)
	cachedRepo := wallet.NewCachedAccountRepo(accountRepo, rdb, log)
	txnRepo := wallet.NewTransactionRepo(pool)
	txManager := wallet.NewPgTransactionManager(pool)

	transferSvc := wallet.NewTransferService(cachedRepo, txnRepo, txManager, log)

	// --- NATS (Event-Driven) ---
	natsClient, err := messaging.NewNATSClient(cfg.NatsURL, log)
	if err != nil {
		log.Warn("NATS not available, events will be disabled", "error", err)
	} else {
		defer natsClient.Close()
		transferSvc.WithPublisher(natsClient)

		// Start notification worker (subscribes to transfer events)
		worker := notification.NewWorker(natsClient, log)
		if err := worker.Start(); err != nil {
			log.Error("failed to start notification worker", "error", err)
		}
	}

	app := fiber.New(fiber.Config{
		AppName:      "GopherWallet v1.0",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New())
	app.Use(middleware.PrometheusMiddleware())
	app.Use(middleware.Idempotency(rdb, 24*time.Hour))

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "gopher-wallet"})
	})

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	handler := wallet.NewHandler(transferSvc, cachedRepo, log)
	handler.RegisterRoutes(app)

	go func() {
		if err := app.Listen(cfg.ServerAddr()); err != nil {
			log.Error("server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")
	if err := app.Shutdown(); err != nil {
		log.Error("server shutdown error", "error", err)
	}
	log.Info("server stopped")
}

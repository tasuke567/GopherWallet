package middleware

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gopher_wallet_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gopher_wallet_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	TransfersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gopher_wallet_transfers_total",
			Help: "Total number of transfer attempts",
		},
		[]string{"status"},
	)

	TransferAmountTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gopher_wallet_transfer_amount_total",
			Help: "Total amount transferred (in smallest currency unit)",
		},
	)
)

func PrometheusMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())

		httpRequestsTotal.WithLabelValues(c.Method(), c.Path(), status).Inc()
		httpRequestDuration.WithLabelValues(c.Method(), c.Path()).Observe(duration)

		return err
	}
}

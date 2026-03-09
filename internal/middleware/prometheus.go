package middleware

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
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

	// Cache metrics
	CacheHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gopher_wallet_cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	CacheMissesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gopher_wallet_cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	// Event publish metrics
	EventPublishTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gopher_wallet_event_publish_total",
			Help: "Total number of event publish attempts",
		},
		[]string{"status"},
	)

	// Circuit breaker metrics
	CircuitBreakerState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gopher_wallet_circuit_breaker_state",
			Help: "Current circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
	)
)

// RegisterDBPoolMetrics exposes pgx connection pool statistics to Prometheus.
func RegisterDBPoolMetrics(pool *pgxpool.Pool) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gopher_wallet_db_pool_total_connections",
		Help: "Total number of connections in the DB pool",
	}, func() float64 { return float64(pool.Stat().TotalConns()) })

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gopher_wallet_db_pool_idle_connections",
		Help: "Number of idle connections in the DB pool",
	}, func() float64 { return float64(pool.Stat().IdleConns()) })

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gopher_wallet_db_pool_acquired_connections",
		Help: "Number of acquired connections in the DB pool",
	}, func() float64 { return float64(pool.Stat().AcquiredConns()) })
}

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

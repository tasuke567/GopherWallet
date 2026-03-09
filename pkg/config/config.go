package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	ServerPort  string
	DatabaseURL string
	RedisURL    string
	NatsURL     string

	// Pool tuning
	DBMaxConns    int32
	DBMinConns    int32
	RedisPoolSize int

	// Resilience
	RateLimit         int // max requests per minute per IP
	RequestTimeoutSec int
	CBMaxFailures     int // circuit breaker failure threshold
	CBTimeoutSec      int // circuit breaker recovery timeout
	WorkerPoolSize    int // notification worker pool size
}

func Load() *Config {
	return &Config{
		ServerPort:  getEnv("SERVER_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gopher_wallet?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "localhost:6379"),
		NatsURL:     getEnv("NATS_URL", "nats://localhost:4222"),

		DBMaxConns:    int32(getEnvInt("DB_MAX_CONNS", 25)),
		DBMinConns:    int32(getEnvInt("DB_MIN_CONNS", 5)),
		RedisPoolSize: getEnvInt("REDIS_POOL_SIZE", 10),

		RateLimit:         getEnvInt("RATE_LIMIT", 100),
		RequestTimeoutSec: getEnvInt("REQUEST_TIMEOUT_SEC", 15),
		CBMaxFailures:     getEnvInt("CB_MAX_FAILURES", 5),
		CBTimeoutSec:      getEnvInt("CB_TIMEOUT_SEC", 30),
		WorkerPoolSize:    getEnvInt("WORKER_POOL_SIZE", 4),
	}
}

func (c *Config) ServerAddr() string {
	return fmt.Sprintf(":%s", c.ServerPort)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

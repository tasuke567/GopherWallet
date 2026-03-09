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
}

func Load() *Config {
	return &Config{
		ServerPort:  getEnv("SERVER_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gopher_wallet?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "localhost:6379"),
		NatsURL:     getEnv("NATS_URL", "nats://localhost:4222"),
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

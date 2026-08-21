package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string
	SessionTTL  time.Duration
	WorkerEvery time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddr:    env("CANALCLEAR_ADDR", ":8080"),
		SessionTTL:  duration("CANALCLEAR_SESSION_TTL", 8*time.Hour),
		WorkerEvery: duration("CANALCLEAR_WORKER_EVERY", 2*time.Second),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

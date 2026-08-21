package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

func (config Config) Validate() error {
	parsed, err := url.Parse(config.DatabaseURL)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" || parsed.Host == "" || strings.TrimPrefix(parsed.Path, "/") == "" {
		return fmt.Errorf("DATABASE_URL must identify a PostgreSQL database")
	}
	if config.HTTPAddr == "" {
		return fmt.Errorf("HTTP address is required")
	}
	if _, port, err := net.SplitHostPort(config.HTTPAddr); err != nil || port == "" {
		return fmt.Errorf("HTTP address must include a port")
	}
	if config.SessionTTL < 5*time.Minute || config.SessionTTL > 24*time.Hour {
		return fmt.Errorf("session TTL must be between 5m and 24h")
	}
	if config.WorkerEvery < 100*time.Millisecond || config.WorkerEvery > time.Hour {
		return fmt.Errorf("worker interval must be between 100ms and 1h")
	}
	return nil
}

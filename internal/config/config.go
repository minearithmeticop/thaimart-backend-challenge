// Package config loads runtime configuration from environment variables
// and validates it up front, so a misconfigured environment fails at
// startup instead of mid-flight.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config groups every runtime knob of the service.
type Config struct {
	HTTPAddr   string
	MongoURI   string
	MongoDB    string
	JWTSecret  string
	JWTTTL     time.Duration
	BcryptCost int
	LogFormat  string // "text" or "json"
}

// Load builds a Config from the environment. Unset values fall back to
// development defaults; values that are set but invalid are startup
// errors.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:   envOr("HTTP_ADDR", ":8080"),
		MongoURI:   envOr("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:    envOr("MONGO_DB", "thaimart"),
		JWTSecret:  os.Getenv("JWT_SECRET"),
		LogFormat:  envOr("LOG_FORMAT", "text"),
	}

	var err error
	if c.JWTTTL, err = envDuration("JWT_TTL", time.Hour); err != nil {
		return Config{}, err
	}
	if c.BcryptCost, err = envInt("BCRYPT_COST", 10); err != nil {
		return Config{}, err
	}

	if c.JWTSecret == "" {
		// Development fallback: mint an ephemeral secret so the service
		// still boots. Tokens die on every restart — production must set
		// JWT_SECRET.
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return Config{}, fmt.Errorf("generating ephemeral JWT secret: %w", err)
		}
		c.JWTSecret = hex.EncodeToString(key)
		slog.Warn("JWT_SECRET not set; generated an ephemeral secret (tokens will not survive restarts)")
	}

	if c.JWTTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_TTL must be positive, got %s", c.JWTTTL)
	}
	if c.BcryptCost < 4 || c.BcryptCost > 15 {
		return Config{}, fmt.Errorf("BCRYPT_COST must be between 4 and 15, got %d", c.BcryptCost)
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return Config{}, fmt.Errorf("LOG_FORMAT must be %q or %q, got %q", "text", "json", c.LogFormat)
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration like 30m or 1h30m, got %q", key, v)
	}
	return d, nil
}

func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	return n, nil
}

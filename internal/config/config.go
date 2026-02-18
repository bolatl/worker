package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort          string
	DatabaseURL       string
	RabbitURL         string
	QueueName         string
	Prefetch          int
	MaxAttempts       int
	ProcessingTimeout time.Duration // used by reaper (stuck processing)
	WorkDuration      time.Duration // simulated work
}

// Load reads environment variables and returns a Config with defaults where unspecified.
func Load() (Config, error) {
	cfg := Config{
		HTTPPort:    getEnv("HTTP_PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),

		RabbitURL: os.Getenv("RABBIT_URL"),
		QueueName: getEnv("QUEUE_NAME", "jobs"),

		Prefetch:    getEnvInt("PREFETCH", 1),
		MaxAttempts: getEnvInt("MAX_ATTEMPTS", 5),

		ProcessingTimeout: getEnvDuration("PROCESSING_TIMEOUT", 60*time.Second),
		WorkDuration:      getEnvDuration("WORK_DURATION", 15*time.Second),
	}

	return cfg, nil
}

// getEnv returns the environment variable value or the default if empty.
func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// getEnvInt parses an environment variable as int or returns the default.
func getEnvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		fmt.Fprintf(os.Stderr, "warning: %s is not set, using default %d\n", k, def)
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid value for %s: %v, using default %d\n", k, err, def)
		return def
	}
	return n
}

// getEnvDuration parses an environment variable as duration or returns the default.
func getEnvDuration(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

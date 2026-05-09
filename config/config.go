package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// LLM
	OpenRouterAPIKey string
	OpenRouterModel  string
	OpenRouterURL    string

	// Restaurant service
	RestaurantServiceURL string

	// Auth — shared with restaurant service to mint InternalJWTs
	InternalJWTSecret  string
	ServiceAccountID   string // UUID used as the AI service's user_id in JWTs

	// mTLS — certs enrolled by start.sh from step-ca (always enabled)
	TLSCACert string
	TLSCert   string
	TLSKey    string

	// Worker
	PollInterval time.Duration
	BatchSize    int

	// Logging
	LogLevel string
}

func Load() *Config {
	pollSec := getEnvAsInt("POLL_INTERVAL_SECONDS", 30)
	batchSize := getEnvAsInt("REVIEW_BATCH_SIZE", 10)

	return &Config{
		OpenRouterAPIKey: getEnv("OPENROUTER_API_KEY", ""),
		OpenRouterModel:  getEnv("OPENROUTER_MODEL", "meta-llama/llama-3.1-8b-instruct:free"),
		OpenRouterURL:    getEnv("OPENROUTER_URL", "https://openrouter.ai/api/v1/chat/completions"),

		RestaurantServiceURL: getEnv("RESTAURANT_SERVICE_URL", "https://sys-backend-restaurant-n-shopping.serveyourstay.com:8443"),

		InternalJWTSecret: getEnv("INTERNAL_JWT_SECRET", ""),
		ServiceAccountID:  getEnv("AI_SERVICE_ACCOUNT_ID", "00000000-0000-0000-0000-000000000001"),

		TLSCACert: getEnv("TLS_CA_CERT", "/etc/certs/ca.crt"),
		TLSCert:   getEnv("TLS_CERT", "/etc/certs/client.crt"),
		TLSKey:    getEnv("TLS_KEY", "/etc/certs/client.key"),

		PollInterval: time.Duration(pollSec) * time.Second,
		BatchSize:    batchSize,

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

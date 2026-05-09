package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"sys-ai/client"
	"sys-ai/config"
	"sys-ai/llm"
	ailogger "sys-ai/logger"
	"sys-ai/reviewer"

	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	ailogger.Init(cfg.LogLevel)
	logger := ailogger.Get()

	if cfg.OpenRouterAPIKey == "" {
		log.Fatal("OPENROUTER_API_KEY is required")
	}
	if cfg.InternalJWTSecret == "" {
		log.Fatal("INTERNAL_JWT_SECRET is required")
	}
	if cfg.RestaurantServiceURL == "" {
		log.Fatal("RESTAURANT_SERVICE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// LLM client
	llmClient := llm.NewClient(cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterURL)
	logger.Info("LLM client initialized", zap.String("model", cfg.OpenRouterModel))

	// Restaurant service client
	restaurantClient, err := client.NewRestaurantClient(client.ClientConfig{
		BaseURL:           cfg.RestaurantServiceURL,
		InternalJWTSecret: cfg.InternalJWTSecret,
		ServiceAccountID:  cfg.ServiceAccountID,
		CACertFile:        cfg.TLSCACert,
		CertFile:          cfg.TLSCert,
		KeyFile:           cfg.TLSKey,
	})
	if err != nil {
		log.Fatalf("failed to create restaurant client: %v", err)
	}
	logger.Info("restaurant service client initialized",
		zap.String("url", cfg.RestaurantServiceURL),
		zap.Bool("tls_enabled", cfg.TLSEnabled),
	)

	// Contribution reviewer worker
	r := reviewer.New(restaurantClient, llmClient, cfg.BatchSize, logger)

	r.Run(ctx, cfg.PollInterval)

	logger.Info("sys-ai shut down cleanly")
}

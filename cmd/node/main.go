package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mikimiya/remnawave-node/internal/config"
	"github.com/Mikimiya/remnawave-node/internal/server"
	"github.com/Mikimiya/remnawave-node/internal/services"
	"github.com/Mikimiya/remnawave-node/pkg/logger"
	"github.com/joho/godotenv"

	_ "github.com/xtls/xray-core/main/distro/all"
)

var (
	Version   = "1.0.2"
	BuildTime = "unknown"
)

func main() {
	// Initialize logger
	log := logger.New()
	defer log.Sync()

	// Load .env file
	_ = godotenv.Load()

	// Set node version for API responses
	services.SetNodeVersion(Version)

	log.Info("Starting Remnawave Node",
		"version", Version,
		"buildTime", BuildTime,
	)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", "error", err)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and start server
	srv, err := server.New(cfg, log)
	if err != nil {
		log.Fatal("Failed to create server", "error", err)
	}

	// Start server in goroutine
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("Server error", "error", err)
			cancel()
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Info("Shutdown signal received")
	case <-ctx.Done():
		log.Info("Context cancelled")
	}

	// Graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server shutdown error", "error", err)
	}

	log.Info("Server stopped")
}

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Mikimiya/remnawave-node/pkg/crypto"
)

// Config holds all configuration values
type Config struct {
	// Server settings
	NodePort int

	// Secret key (contains TLS certs and JWT public key)
	SecretKey string

	// Parsed payload from SECRET_KEY
	NodePayload *crypto.NodePayload

	// Feature flags
	DisableHashedSetCheck bool

	// Port mapping for NAT machines (format: "originalPort:mappedPort,originalPort:mappedPort")
	PortMap map[int]int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}

	// NODE_PORT (required)
	portStr := getEnv("NODE_PORT", "3000")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid NODE_PORT: %w", err)
	}
	cfg.NodePort = port

	// SECRET_KEY (required)
	cfg.SecretKey = os.Getenv("SECRET_KEY")
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("SECRET_KEY is required")
	}

	// Parse SECRET_KEY payload
	payload, err := crypto.ParseNodePayload(cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SECRET_KEY: %w", err)
	}
	cfg.NodePayload = payload

	// Feature flags
	cfg.DisableHashedSetCheck = getEnvBool("DISABLE_HASHED_SET_CHECK", false)

	// Port mapping for NAT machines
	portMap, err := parsePortMap(os.Getenv("PORT_MAP"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT_MAP: %w", err)
	}
	cfg.PortMap = portMap

	return cfg, nil
}

// getEnv returns environment variable value or default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool returns environment variable as bool or default
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

// parsePortMap parses PORT_MAP environment variable
// Format: "originalPort:mappedPort,originalPort:mappedPort"
// Example: "443:10000,80:10001,8443:10002"
func parsePortMap(value string) (map[int]int, error) {
	if value == "" {
		return nil, nil
	}

	portMap := make(map[int]int)
	pairs := strings.Split(value, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid port mapping format: %q (expected original:mapped)", pair)
		}

		original, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid original port %q: %w", parts[0], err)
		}

		mapped, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid mapped port %q: %w", parts[1], err)
		}

		if original < 1 || original > 65535 || mapped < 1 || mapped > 65535 {
			return nil, fmt.Errorf("port out of range (1-65535): %d:%d", original, mapped)
		}

		portMap[original] = mapped
	}

	return portMap, nil
}

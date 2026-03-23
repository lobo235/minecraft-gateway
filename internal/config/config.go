package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	NFSBasePath     string
	NomadGatewayURL string
	NomadGatewayKey string
	VaultGatewayURL string
	VaultGatewayKey string
	GatewayAPIKey   string
	Port            string
	LogLevel        string
	DataDir         string
}

// Load reads configuration from environment variables, applying defaults and validating required fields.
func Load() (*Config, error) {
	// Load .env if present — ignore error if file doesn't exist.
	_ = godotenv.Load()

	cfg := &Config{
		NFSBasePath:     os.Getenv("NFS_BASE_PATH"),
		NomadGatewayURL: os.Getenv("NOMAD_GATEWAY_URL"),
		NomadGatewayKey: os.Getenv("NOMAD_GATEWAY_KEY"),
		VaultGatewayURL: os.Getenv("VAULT_GATEWAY_URL"),
		VaultGatewayKey: os.Getenv("VAULT_GATEWAY_KEY"),
		GatewayAPIKey:   os.Getenv("GATEWAY_API_KEY"),
		Port:            os.Getenv("PORT"),
		LogLevel:        os.Getenv("LOG_LEVEL"),
		DataDir:         os.Getenv("DATA_DIR"),
	}

	if cfg.NFSBasePath == "" {
		return nil, fmt.Errorf("NFS_BASE_PATH is required")
	}
	if cfg.NomadGatewayURL == "" {
		return nil, fmt.Errorf("NOMAD_GATEWAY_URL is required")
	}
	if cfg.NomadGatewayKey == "" {
		return nil, fmt.Errorf("NOMAD_GATEWAY_KEY is required")
	}
	if cfg.VaultGatewayURL == "" {
		return nil, fmt.Errorf("VAULT_GATEWAY_URL is required")
	}
	if cfg.VaultGatewayKey == "" {
		return nil, fmt.Errorf("VAULT_GATEWAY_KEY is required")
	}
	if cfg.GatewayAPIKey == "" {
		return nil, fmt.Errorf("GATEWAY_API_KEY is required")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/data"
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	case "":
		cfg.LogLevel = "info"
	default:
		return nil, fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}

	return cfg, nil
}

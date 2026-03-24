package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	NFSBasePath      string
	NomadGatewayURL  string
	NomadGatewayKey  string
	VaultGatewayURL  string
	VaultGatewayKey  string
	GatewayAPIKey    string
	Port             string
	LogLevel         string
	DataDir          string
	MaxDownloadSize  int64
	MaxWriteFileSize int64
	MaxExtractSize   int64
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

	// Parse MAX_DOWNLOAD_SIZE (default 2GB).
	maxDL := os.Getenv("MAX_DOWNLOAD_SIZE")
	if maxDL == "" {
		cfg.MaxDownloadSize = 2147483648 // 2GB
	} else {
		v, err := strconv.ParseInt(maxDL, 10, 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("MAX_DOWNLOAD_SIZE must be a positive integer")
		}
		cfg.MaxDownloadSize = v
	}

	// Parse MAX_WRITE_FILE_SIZE (default 1MB).
	maxWF := os.Getenv("MAX_WRITE_FILE_SIZE")
	if maxWF == "" {
		cfg.MaxWriteFileSize = 1048576 // 1MB
	} else {
		v, err := strconv.ParseInt(maxWF, 10, 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("MAX_WRITE_FILE_SIZE must be a positive integer")
		}
		cfg.MaxWriteFileSize = v
	}

	// Parse MAX_EXTRACT_SIZE (default 10GB).
	maxES := os.Getenv("MAX_EXTRACT_SIZE")
	if maxES == "" {
		cfg.MaxExtractSize = 10737418240 // 10GB
	} else {
		v, err := strconv.ParseInt(maxES, 10, 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("MAX_EXTRACT_SIZE must be a positive integer")
		}
		cfg.MaxExtractSize = v
	}

	return cfg, nil
}

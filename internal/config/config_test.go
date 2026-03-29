package config

import (
	"os"
	"testing"
)

func clearEnv() {
	for _, key := range []string{
		"NOMAD_GATEWAY_URL", "NOMAD_GATEWAY_KEY",
		"VAULT_GATEWAY_URL", "VAULT_GATEWAY_KEY", "GATEWAY_API_KEY",
		"PORT", "LOG_LEVEL",
	} {
		os.Unsetenv(key)
	}
}

func setRequiredEnv() {
	os.Setenv("NOMAD_GATEWAY_URL", "http://localhost:8081")
	os.Setenv("NOMAD_GATEWAY_KEY", "nomad-key")
	os.Setenv("VAULT_GATEWAY_URL", "http://localhost:8082")
	os.Setenv("VAULT_GATEWAY_KEY", "vault-key")
	os.Setenv("GATEWAY_API_KEY", "test-api-key")
}

func TestLoadSuccess(t *testing.T) {
	clearEnv()
	setRequiredEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	tests := []struct {
		name   string
		unset  string
		errMsg string
	}{
		{"missing NOMAD_GATEWAY_URL", "NOMAD_GATEWAY_URL", "NOMAD_GATEWAY_URL is required"},
		{"missing NOMAD_GATEWAY_KEY", "NOMAD_GATEWAY_KEY", "NOMAD_GATEWAY_KEY is required"},
		{"missing VAULT_GATEWAY_URL", "VAULT_GATEWAY_URL", "VAULT_GATEWAY_URL is required"},
		{"missing VAULT_GATEWAY_KEY", "VAULT_GATEWAY_KEY", "VAULT_GATEWAY_KEY is required"},
		{"missing GATEWAY_API_KEY", "GATEWAY_API_KEY", "GATEWAY_API_KEY is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv()
			setRequiredEnv()
			os.Unsetenv(tt.unset)

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tt.errMsg {
				t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	os.Setenv("LOG_LEVEL", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
}

func TestLoadCustomValues(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	os.Setenv("PORT", "9090")
	os.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

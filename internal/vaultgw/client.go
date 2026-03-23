package vaultgw

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client is the interface for communicating with vault-gateway.
type Client interface {
	// GetRCONPassword retrieves the RCON password for a Minecraft server.
	GetRCONPassword(serverName string) (string, error)
}

// secretResponse is the expected response from vault-gateway GET /secrets/minecraft/{name}.
type secretResponse struct {
	RCONPassword string `json:"rcon_password"`
}

type client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *slog.Logger
}

// NewClient creates a new vault-gateway HTTP client.
func NewClient(baseURL, apiKey string, log *slog.Logger) Client {
	return &client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		log: log,
	}
}

// GetRCONPassword calls GET /secrets/minecraft/{serverName} on vault-gateway.
func (c *client) GetRCONPassword(serverName string) (string, error) {
	url := fmt.Sprintf("%s/secrets/minecraft/%s", c.baseURL, serverName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault-gateway request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault-gateway returned %d: %s", resp.StatusCode, string(body))
	}

	var secret secretResponse
	if err := json.Unmarshal(body, &secret); err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	if secret.RCONPassword == "" {
		return "", fmt.Errorf("vault-gateway returned empty RCON password for %s", serverName)
	}
	return secret.RCONPassword, nil
}

package nomadgw

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client is the interface for communicating with nomad-gateway.
type Client interface {
	// GetAllocations retrieves allocations for a job, returning the RCON port and host.
	GetAllocations(jobName string) ([]Allocation, error)
}

// Allocation holds minimal allocation info needed for RCON resolution.
type Allocation struct {
	ID     string        `json:"ID"`
	Status string        `json:"ClientStatus"`
	Ports  []PortMapping `json:"AllocatedPorts"`
	NodeID string        `json:"NodeID"`
}

// PortMapping represents a port mapping from a Nomad allocation.
type PortMapping struct {
	Label  string `json:"Label"`
	Value  int    `json:"Value"`
	To     int    `json:"To"`
	HostIP string `json:"HostIP"`
}

type client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *slog.Logger
}

// NewClient creates a new nomad-gateway HTTP client.
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

// GetAllocations calls GET /jobs/{jobName}/allocations on the nomad-gateway.
func (c *client) GetAllocations(jobName string) ([]Allocation, error) {
	url := fmt.Sprintf("%s/jobs/%s/allocations", c.baseURL, jobName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nomad-gateway request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nomad-gateway returned %d: %s", resp.StatusCode, string(body))
	}

	var allocs []Allocation
	if err := json.Unmarshal(body, &allocs); err != nil {
		return nil, fmt.Errorf("decode allocations: %w", err)
	}
	return allocs, nil
}

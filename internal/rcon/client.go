package rcon

import (
	"fmt"
	"log/slog"

	gorcon "github.com/gorcon/rcon"

	"github.com/lobo235/minecraft-gateway/internal/nomadgw"
)

// NomadClient is the interface for resolving server allocations.
type NomadClient interface {
	GetAllocations(jobName string) ([]nomadgw.Allocation, error)
}

// VaultClient is the interface for retrieving RCON passwords.
type VaultClient interface {
	GetRCONPassword(serverName string) (string, error)
}

// Client provides RCON command execution against Minecraft servers.
type Client interface {
	// Execute sends an RCON command to a named server and returns the response.
	Execute(serverName, command string) (string, error)
}

type client struct {
	nomad NomadClient
	vault VaultClient
	log   *slog.Logger
}

// NewClient creates a new RCON client that resolves connection details from upstream gateways.
func NewClient(nomad NomadClient, vault VaultClient, log *slog.Logger) Client {
	return &client{nomad: nomad, vault: vault, log: log}
}

// Execute resolves the RCON host/port and password for a server, then sends the command.
func (c *client) Execute(serverName, command string) (string, error) {
	host, port, err := c.resolveRCON(serverName)
	if err != nil {
		return "", fmt.Errorf("resolve rcon endpoint: %w", err)
	}

	password, err := c.vault.GetRCONPassword(serverName)
	if err != nil {
		return "", fmt.Errorf("get rcon password: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := gorcon.Dial(addr, password)
	if err != nil {
		return "", fmt.Errorf("rcon connect to %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := conn.Execute(command)
	if err != nil {
		return "", fmt.Errorf("rcon execute: %w", err)
	}
	return resp, nil
}

// resolveRCON finds the RCON host and port from the running allocation.
func (c *client) resolveRCON(serverName string) (string, int, error) {
	allocs, err := c.nomad.GetAllocations(serverName)
	if err != nil {
		return "", 0, fmt.Errorf("get allocations: %w", err)
	}

	// Find a running allocation with an rcon port.
	for _, alloc := range allocs {
		if alloc.Status != "running" {
			continue
		}
		for _, port := range alloc.Ports {
			if port.Label == "rcon" {
				return port.HostIP, port.Value, nil
			}
		}
	}
	return "", 0, fmt.Errorf("no running allocation with rcon port found for %s", serverName)
}

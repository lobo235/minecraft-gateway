package rcon

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/lobo235/minecraft-gateway/internal/nomadgw"
)

// --- Mock NomadClient ---

type mockNomadClient struct {
	allocations    []nomadgw.Allocation
	allocationsErr error
	allocation     *nomadgw.Allocation
	allocationErr  error
}

func (m *mockNomadClient) GetAllocations(_ string) ([]nomadgw.Allocation, error) {
	return m.allocations, m.allocationsErr
}

func (m *mockNomadClient) GetAllocation(_, _ string) (*nomadgw.Allocation, error) {
	return m.allocation, m.allocationErr
}

// --- Mock VaultClient ---

type mockVaultClient struct {
	password string
	err      error
}

func (m *mockVaultClient) GetRCONPassword(_ string) (string, error) {
	return m.password, m.err
}

// --- Helpers ---

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// --- resolveRCON tests ---

func TestResolveRCON_Success(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-1", Status: "running"},
		},
		allocation: &nomadgw.Allocation{
			ID:     "alloc-1",
			Status: "running",
			AllocatedResources: &nomadgw.AllocatedResources{
				Shared: nomadgw.SharedResources{
					Ports: []nomadgw.PortMapping{
						{Label: "minecraft", Value: 25565, To: 25565, HostIP: "10.0.0.1"},
						{Label: "rcon", Value: 25575, To: 25575, HostIP: "10.0.0.1"},
					},
				},
			},
		},
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	host, port, err := c.resolveRCON("mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "10.0.0.1" {
		t.Errorf("host = %q, want 10.0.0.1", host)
	}
	if port != 25575 {
		t.Errorf("port = %d, want 25575", port)
	}
}

func TestResolveRCON_NoRunningAllocs(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-1", Status: "complete"},
			{ID: "alloc-2", Status: "failed"},
		},
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	_, _, err := c.resolveRCON("mc-test")
	if err == nil {
		t.Fatal("expected error for no running allocs")
	}
	if want := "no running allocation with rcon port found for mc-test"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveRCON_EmptyAllocations(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{},
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	_, _, err := c.resolveRCON("mc-test")
	if err == nil {
		t.Fatal("expected error for empty allocations")
	}
}

func TestResolveRCON_GetAllocationsError(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocationsErr: errors.New("nomad unavailable"),
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	_, _, err := c.resolveRCON("mc-test")
	if err == nil {
		t.Fatal("expected error when GetAllocations fails")
	}
	if !errors.Is(err, nomadMock.allocationsErr) {
		t.Errorf("error should wrap original: %v", err)
	}
}

func TestResolveRCON_GetAllocationError(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-1", Status: "running"},
		},
		allocationErr: errors.New("alloc fetch failed"),
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	_, _, err := c.resolveRCON("mc-test")
	if err == nil {
		t.Fatal("expected error when single running alloc fails to fetch details")
	}
}

func TestResolveRCON_NoRCONPort(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-1", Status: "running"},
		},
		allocation: &nomadgw.Allocation{
			ID:     "alloc-1",
			Status: "running",
			AllocatedResources: &nomadgw.AllocatedResources{
				Shared: nomadgw.SharedResources{
					Ports: []nomadgw.PortMapping{
						{Label: "minecraft", Value: 25565, To: 25565, HostIP: "10.0.0.1"},
					},
				},
			},
		},
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	_, _, err := c.resolveRCON("mc-test")
	if err == nil {
		t.Fatal("expected error when no rcon port found")
	}
}

func TestResolveRCON_NilAllocatedResources(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-1", Status: "running"},
		},
		allocation: &nomadgw.Allocation{
			ID:                 "alloc-1",
			Status:             "running",
			AllocatedResources: nil,
		},
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	_, _, err := c.resolveRCON("mc-test")
	if err == nil {
		t.Fatal("expected error when AllocatedResources is nil")
	}
}

func TestResolveRCON_SkipsNonRunningThenFindsRunning(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-old", Status: "complete"},
			{ID: "alloc-running", Status: "running"},
		},
		allocation: &nomadgw.Allocation{
			ID:     "alloc-running",
			Status: "running",
			AllocatedResources: &nomadgw.AllocatedResources{
				Shared: nomadgw.SharedResources{
					Ports: []nomadgw.PortMapping{
						{Label: "rcon", Value: 25575, To: 25575, HostIP: "10.0.0.2"},
					},
				},
			},
		},
	}

	c := &client{nomad: nomadMock, vault: &mockVaultClient{}, log: testLogger()}
	host, port, err := c.resolveRCON("mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "10.0.0.2" || port != 25575 {
		t.Errorf("got %s:%d, want 10.0.0.2:25575", host, port)
	}
}

// --- Execute tests ---

func TestExecute_ResolveError(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocationsErr: errors.New("nomad down"),
	}
	c := NewClient(nomadMock, &mockVaultClient{}, testLogger())

	_, err := c.Execute("mc-test", "say hello")
	if err == nil {
		t.Fatal("expected error when resolve fails")
	}
	if want := "resolve rcon endpoint"; !errors.Is(err, nil) && !containsStr(err.Error(), want) {
		t.Errorf("error should contain %q, got %q", want, err.Error())
	}
}

func TestExecute_VaultError(t *testing.T) {
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-1", Status: "running"},
		},
		allocation: &nomadgw.Allocation{
			ID:     "alloc-1",
			Status: "running",
			AllocatedResources: &nomadgw.AllocatedResources{
				Shared: nomadgw.SharedResources{
					Ports: []nomadgw.PortMapping{
						{Label: "rcon", Value: 25575, To: 25575, HostIP: "10.0.0.1"},
					},
				},
			},
		},
	}
	vaultMock := &mockVaultClient{err: errors.New("vault sealed")}
	c := NewClient(nomadMock, vaultMock, testLogger())

	_, err := c.Execute("mc-test", "say hello")
	if err == nil {
		t.Fatal("expected error when vault fails")
	}
	if !containsStr(err.Error(), "get rcon password") {
		t.Errorf("error should contain 'get rcon password', got %q", err.Error())
	}
}

func TestExecute_DialError(t *testing.T) {
	// gorcon.Dial will fail because there's nothing listening on this address.
	nomadMock := &mockNomadClient{
		allocations: []nomadgw.Allocation{
			{ID: "alloc-1", Status: "running"},
		},
		allocation: &nomadgw.Allocation{
			ID:     "alloc-1",
			Status: "running",
			AllocatedResources: &nomadgw.AllocatedResources{
				Shared: nomadgw.SharedResources{
					Ports: []nomadgw.PortMapping{
						{Label: "rcon", Value: 19999, To: 25575, HostIP: "127.0.0.1"},
					},
				},
			},
		},
	}
	vaultMock := &mockVaultClient{password: "test-password"}
	c := NewClient(nomadMock, vaultMock, testLogger())

	_, err := c.Execute("mc-test", "say hello")
	if err == nil {
		t.Fatal("expected error when dial fails (nothing listening)")
	}
	if !containsStr(err.Error(), "rcon connect") {
		t.Errorf("error should contain 'rcon connect', got %q", err.Error())
	}
}

// --- NewClient ---

func TestNewClient(t *testing.T) {
	c := NewClient(&mockNomadClient{}, &mockVaultClient{}, testLogger())
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
}

// --- Helper ---

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

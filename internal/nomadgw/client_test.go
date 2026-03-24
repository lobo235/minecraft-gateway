package nomadgw

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGetAllocations_Success(t *testing.T) {
	allocs := []Allocation{
		{
			ID:     "alloc-123",
			Status: "running",
			AllocatedResources: &AllocatedResources{
				Shared: SharedResources{
					Ports: []PortMapping{
						{Label: "minecraft", Value: 25565, To: 25565, HostIP: "10.0.0.1"},
						{Label: "rcon", Value: 25575, To: 25575, HostIP: "10.0.0.1"},
					},
				},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/mc-test/allocations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or incorrect authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(allocs)
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	result, err := c.GetAllocations("mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(result))
	}
	if result[0].ID != "alloc-123" {
		t.Errorf("alloc ID = %q, want alloc-123", result[0].ID)
	}
	ports := result[0].GetPorts()
	if len(ports) != 2 {
		t.Errorf("expected 2 ports, got %d", len(ports))
	}
}

func TestGetAllocations_Upstream4xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":"not_found","message":"job not found"}`))
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	_, err := c.GetAllocations("nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestGetAllocations_Upstream5xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	_, err := c.GetAllocations("test")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetAllocations_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	_, err := c.GetAllocations("test")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestGetAllocations_NetworkError(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	// Use a URL that won't connect.
	c := NewClient("http://127.0.0.1:1", "test-key", log)

	_, err := c.GetAllocations("test")
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

// --- GetAllocation tests ---

func TestGetAllocation_Success(t *testing.T) {
	alloc := Allocation{
		ID:     "alloc-456",
		Status: "running",
		AllocatedResources: &AllocatedResources{
			Shared: SharedResources{
				Ports: []PortMapping{
					{Label: "minecraft", Value: 25565, To: 25565, HostIP: "10.0.0.1"},
					{Label: "rcon", Value: 25575, To: 25575, HostIP: "10.0.0.1"},
				},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/mc-test/allocations/alloc-456" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or incorrect authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(alloc)
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	result, err := c.GetAllocation("mc-test", "alloc-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "alloc-456" {
		t.Errorf("alloc ID = %q, want alloc-456", result.ID)
	}
	ports := result.GetPorts()
	if len(ports) != 2 {
		t.Errorf("expected 2 ports, got %d", len(ports))
	}
}

func TestGetAllocation_Upstream404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":"not_found","message":"allocation not found"}`))
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	_, err := c.GetAllocation("mc-test", "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestGetAllocation_Upstream500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	_, err := c.GetAllocation("mc-test", "alloc-1")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetAllocation_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "test-key", log)

	_, err := c.GetAllocation("mc-test", "alloc-1")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestGetAllocation_NetworkError(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient("http://127.0.0.1:1", "test-key", log)

	_, err := c.GetAllocation("mc-test", "alloc-1")
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

// --- GetPorts edge case ---

func TestGetPorts_NilAllocatedResources(t *testing.T) {
	alloc := &Allocation{
		ID:                 "alloc-1",
		Status:             "running",
		AllocatedResources: nil,
	}
	ports := alloc.GetPorts()
	if ports != nil {
		t.Errorf("expected nil ports, got %v", ports)
	}
}

func TestGetPorts_EmptyPorts(t *testing.T) {
	alloc := &Allocation{
		ID:     "alloc-1",
		Status: "running",
		AllocatedResources: &AllocatedResources{
			Shared: SharedResources{
				Ports: []PortMapping{},
			},
		},
	}
	ports := alloc.GetPorts()
	if len(ports) != 0 {
		t.Errorf("expected 0 ports, got %d", len(ports))
	}
}

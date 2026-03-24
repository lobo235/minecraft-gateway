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

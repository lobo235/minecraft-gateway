package vaultgw

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGetRCONPassword_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/secrets/minecraft/mc-test" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer vault-key" {
			t.Error("missing or incorrect authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(secretResponse{RCONPassword: "supersecret"})
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "vault-key", log)

	pw, err := c.GetRCONPassword("mc-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pw != "supersecret" {
		t.Errorf("password = %q, want supersecret", pw)
	}
}

func TestGetRCONPassword_EmptyPassword(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(secretResponse{RCONPassword: ""})
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "vault-key", log)

	_, err := c.GetRCONPassword("mc-test")
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestGetRCONPassword_Upstream404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":"not_found","message":"secret not found"}`))
	}))
	defer ts.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	c := NewClient(ts.URL, "vault-key", log)

	_, err := c.GetRCONPassword("nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

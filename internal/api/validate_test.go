package api

import "testing"

func TestValidServerName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"valid lowercase", "mc-test", true},
		{"valid with numbers", "mc1-test2", true},
		{"valid short", "a", true},
		{"invalid uppercase", "MC-TEST", false},
		{"invalid special chars", "mc_test!", false},
		{"invalid starts with dash", "-mc-test", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validServerName(tt.input)
			if got != tt.valid {
				t.Errorf("validServerName(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestValidPlayerName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"valid name", "Steve", true},
		{"valid with underscore", "Player_1", true},
		{"valid all digits", "1234567890123456", true},
		{"invalid too long", "12345678901234567", false},
		{"invalid special chars", "Player!", false},
		{"invalid spaces", "Player Name", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validPlayerName(tt.input)
			if got != tt.valid {
				t.Errorf("validPlayerName(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

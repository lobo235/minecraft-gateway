package api

import "testing"

func TestValidDownloadURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{"forgecdn edge", "https://edge.forgecdn.net/files/1234/mod.jar", true},
		{"forgecdn mediafilez", "https://mediafilez.forgecdn.net/files/1234/mod.jar", true},
		{"github raw lobo235", "https://raw.githubusercontent.com/lobo235/repo/main/file.txt", true},
		{"github lobo235", "https://github.com/lobo235/repo/releases/download/v1/file.jar", true},
		{"modrinth cdn", "https://cdn.modrinth.com/data/abc/versions/1.0/mod.jar", true},
		{"feed-the-beast", "https://feed-the-beast.com/modpacks/download/pack.zip", true},
		{"api feed-the-beast", "https://api.feed-the-beast.com/v1/packs/123", true},
		{"api modpacks ch", "https://api.modpacks.ch/public/modpack/123", true},
		{"github other user", "https://github.com/otheruser/repo/releases/download/v1/file.jar", false},
		{"raw github other user", "https://raw.githubusercontent.com/otheruser/repo/main/file.txt", false},
		{"http not https", "http://edge.forgecdn.net/files/1234/mod.jar", false},
		{"evil domain", "https://evil.com/malware.zip", false},
		{"empty string", "", false},
		{"not a url", "not-a-url", false},
		{"ftp scheme", "ftp://edge.forgecdn.net/file.zip", false},
		{"forgecdn with port", "https://edge.forgecdn.net:8080/files/mod.jar", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validDownloadURL(tt.url)
			if got != tt.valid {
				t.Errorf("validDownloadURL(%q) = %v, want %v", tt.url, got, tt.valid)
			}
		})
	}
}

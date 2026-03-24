package api

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	serverNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	playerNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{1,16}$`)
)

// allowedDownloadHosts lists the exact hostnames (and optional path prefixes)
// permitted for file downloads.
var allowedDownloadHosts = []struct {
	host       string
	pathPrefix string // empty means any path on this host is allowed
}{
	{"edge.forgecdn.net", ""},
	{"mediafilez.forgecdn.net", ""},
	{"raw.githubusercontent.com", "/lobo235/"},
	{"github.com", "/lobo235/"},
	{"cdn.modrinth.com", ""},
	{"feed-the-beast.com", ""},
	{"api.feed-the-beast.com", ""},
	{"api.modpacks.ch", ""},
}

// validServerName checks if the server name matches the allowed pattern.
func validServerName(name string) bool {
	return serverNameRegex.MatchString(name)
}

// validPlayerName checks if the player name matches the allowed pattern.
func validPlayerName(name string) bool {
	return playerNameRegex.MatchString(name)
}

// validDownloadURL checks that the URL is an allowed HTTPS download source.
func validDownloadURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	for _, allowed := range allowedDownloadHosts {
		if strings.EqualFold(u.Host, allowed.host) {
			if allowed.pathPrefix == "" {
				return true
			}
			if strings.HasPrefix(u.Path, allowed.pathPrefix) {
				return true
			}
		}
	}
	return false
}

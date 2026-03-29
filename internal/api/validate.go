package api

import "regexp"

var (
	serverNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	playerNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{1,16}$`)
)

// validServerName checks if the server name matches the allowed pattern.
func validServerName(name string) bool {
	return serverNameRegex.MatchString(name)
}

// validPlayerName checks if the player name matches the allowed pattern.
func validPlayerName(name string) bool {
	return playerNameRegex.MatchString(name)
}

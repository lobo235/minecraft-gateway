package api

import (
	"net/http"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// healthHandler returns a handler for GET /health.
// This endpoint intentionally skips Bearer auth — it is called by Nomad's health check.
func (s *Server) healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.version})
	}
}

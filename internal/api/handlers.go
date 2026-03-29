package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

const (
	maxJSONBodySize = 64 * 1024 // 64KB for most JSON request bodies
)

// limitBody wraps r.Body with http.MaxBytesReader to enforce a body size limit.
func limitBody(w http.ResponseWriter, r *http.Request, maxBytes int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
}

// --- RCON handlers ---

func (s *Server) rconHandler() http.HandlerFunc {
	type request struct {
		Command string `json:"command"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		limitBody(w, r, maxJSONBodySize)
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if req.Command == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "command is required")
			return
		}
		resp, err := s.rcon.Execute(name, req.Command)
		if err != nil {
			s.log.Error("rcon execute failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			if strings.Contains(err.Error(), "no running allocation") {
				writeError(w, http.StatusNotFound, "not_found", "no running server found")
				return
			}
			writeError(w, http.StatusBadGateway, "upstream_error", "RCON command failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"response": resp})
	}
}

func (s *Server) rconOpHandler() http.HandlerFunc {
	type request struct {
		Player string `json:"player"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		limitBody(w, r, maxJSONBodySize)
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if !validPlayerName(req.Player) {
			writeError(w, http.StatusBadRequest, "invalid_body", "player name must match ^[a-zA-Z0-9_]{1,16}$")
			return
		}
		resp, err := s.rcon.Execute(name, "op "+req.Player)
		if err != nil {
			s.log.Error("rcon op failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusBadGateway, "upstream_error", "RCON op command failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"response": resp})
	}
}

func (s *Server) rconDeopHandler() http.HandlerFunc {
	type request struct {
		Player string `json:"player"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		limitBody(w, r, maxJSONBodySize)
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if !validPlayerName(req.Player) {
			writeError(w, http.StatusBadRequest, "invalid_body", "player name must match ^[a-zA-Z0-9_]{1,16}$")
			return
		}
		resp, err := s.rcon.Execute(name, "deop "+req.Player)
		if err != nil {
			s.log.Error("rcon deop failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusBadGateway, "upstream_error", "RCON deop command failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"response": resp})
	}
}

func (s *Server) rconWhitelistHandler() http.HandlerFunc {
	type request struct {
		Action string `json:"action"`
		Player string `json:"player"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		limitBody(w, r, maxJSONBodySize)
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if req.Action != "add" && req.Action != "remove" {
			writeError(w, http.StatusBadRequest, "invalid_body", "action must be 'add' or 'remove'")
			return
		}
		if !validPlayerName(req.Player) {
			writeError(w, http.StatusBadRequest, "invalid_body", "player name must match ^[a-zA-Z0-9_]{1,16}$")
			return
		}
		resp, err := s.rcon.Execute(name, "whitelist "+req.Action+" "+req.Player)
		if err != nil {
			s.log.Error("rcon whitelist failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusBadGateway, "upstream_error", "RCON whitelist command failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"response": resp})
	}
}

func (s *Server) rconPlayersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		resp, err := s.rcon.Execute(name, "list")
		if err != nil {
			s.log.Error("rcon players failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			if strings.Contains(err.Error(), "no running allocation") {
				writeError(w, http.StatusNotFound, "not_found", "no running server found")
				return
			}
			writeError(w, http.StatusBadGateway, "upstream_error", "RCON list command failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"response": resp})
	}
}

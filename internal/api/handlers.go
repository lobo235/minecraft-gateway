package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lobo235/minecraft-gateway/internal/nfs"
)

// --- Server management handlers ---

func (s *Server) listServersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		servers, err := s.nfs.ListServers()
		if err != nil {
			s.log.Error("list servers failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list servers")
			return
		}
		if servers == nil {
			servers = []string{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
	}
}

func (s *Server) createServerHandler() http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
		UID  int    `json:"uid"`
		GID  int    `json:"gid"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "name is required")
			return
		}
		if !validServerName(req.Name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "server name must match ^[a-z0-9][a-z0-9-]{0,47}$")
			return
		}
		if err := s.nfs.CreateServer(req.Name, req.UID, req.GID); err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			s.log.Error("create server failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create server")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name, "status": "created"})
	}
}

func (s *Server) deleteServerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		if r.URL.Query().Get("confirm") != "true" {
			writeError(w, http.StatusBadRequest, "missing_fields", "confirm=true query parameter is required")
			return
		}
		if err := s.nfs.DeleteServer(name); err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			if strings.Contains(err.Error(), "server not found") {
				writeError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}
			s.log.Error("delete server failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete server")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"name": name, "status": "deleted"})
	}
}

// --- File download ---

func (s *Server) downloadHandler() http.HandlerFunc {
	type request struct {
		URL      string `json:"url"`
		DestPath string `json:"dest_path"`
		Extract  bool   `json:"extract"`
		UID      int    `json:"uid"`
		GID      int    `json:"gid"`
		Mode     string `json:"mode"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "url is required")
			return
		}
		if !validDownloadURL(req.URL) {
			writeError(w, http.StatusBadRequest, "invalid_body", "url domain is not allowed")
			return
		}
		if req.DestPath == "" {
			req.DestPath = "."
		}
		if req.Mode == "" {
			req.Mode = "overwrite"
		}
		if !nfs.ValidDownloadMode(req.Mode) {
			writeError(w, http.StatusBadRequest, "invalid_body", "mode must be one of: overwrite, skip_existing, clean_first")
			return
		}
		result, err := s.nfs.Download(name, req.URL, req.DestPath, req.Extract, req.UID, req.GID, nfs.DownloadMode(req.Mode))
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			s.log.Error("download failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to download file")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"files_count": result.FilesCount,
			"total_bytes": result.TotalBytes,
		})
	}
}

// --- Disk usage ---

func (s *Server) diskUsageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		size, err := s.nfs.DiskUsage(name)
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			s.log.Error("disk usage failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to get disk usage")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"server": name, "bytes": size})
	}
}

// --- File operations ---

func (s *Server) listFilesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		subPath := r.URL.Query().Get("path")
		files, err := s.nfs.ListFiles(name, subPath)
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			s.log.Error("list files failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list files")
			return
		}
		if files == nil {
			files = []nfs.FileEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": files})
	}
}

func (s *Server) readFileHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		subPath := r.URL.Query().Get("path")
		if subPath == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "path query parameter is required")
			return
		}
		data, err := s.nfs.ReadFile(name, subPath)
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			if strings.Contains(err.Error(), "exceeds maximum") {
				writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
				return
			}
			if strings.Contains(err.Error(), "no such file") {
				writeError(w, http.StatusNotFound, "not_found", "file not found")
				return
			}
			s.log.Error("read file failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to read file")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"content": string(data)})
	}
}

func (s *Server) grepFilesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		subPath := r.URL.Query().Get("path")
		pattern := r.URL.Query().Get("pattern")
		if pattern == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "pattern query parameter is required")
			return
		}
		result, err := s.nfs.GrepFiles(name, subPath, pattern)
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			s.log.Error("grep files failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to grep files")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --- Backup operations ---

func (s *Server) listBackupsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		backups, err := s.nfs.ListBackups(name)
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			s.log.Error("list backups failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list backups")
			return
		}
		if backups == nil {
			backups = []nfs.BackupInfo{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"backups": backups})
	}
}

func (s *Server) startBackupHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		id, err := s.nfs.StartBackup(name)
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			if strings.Contains(err.Error(), "server not found") {
				writeError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}
			s.log.Error("start backup failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to start backup")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"server": name, "backup_id": id, "status": "running"})
	}
}

func (s *Server) getBackupStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		backupID := r.PathValue("backupID")
		status, err := s.nfs.GetBackupStatus(name, backupID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, "not_found", "backup not found")
				return
			}
			s.log.Error("get backup status failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to get backup status")
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

// --- Restore and migrate ---

func (s *Server) restoreHandler() http.HandlerFunc {
	type request struct {
		BackupID string `json:"backup_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if req.BackupID == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "backup_id is required")
			return
		}
		if err := s.nfs.Restore(name, req.BackupID); err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			if strings.Contains(err.Error(), "backup not found") {
				writeError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}
			s.log.Error("restore failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to restore backup")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"server": name, "backup_id": req.BackupID, "status": "restored"})
	}
}

func (s *Server) migrateHandler() http.HandlerFunc {
	type request struct {
		NewName string `json:"new_name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if req.NewName == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "new_name is required")
			return
		}
		if !validServerName(req.NewName) {
			writeError(w, http.StatusBadRequest, "invalid_body", "new_name must match ^[a-z0-9][a-z0-9-]{0,47}$")
			return
		}
		if err := s.nfs.Migrate(name, req.NewName); err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			if strings.Contains(err.Error(), "server not found") {
				writeError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}
			if strings.Contains(err.Error(), "already exists") {
				writeError(w, http.StatusConflict, "conflict", err.Error())
				return
			}
			s.log.Error("migrate failed", "error", err, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to migrate server")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"old_name": name, "new_name": req.NewName, "status": "migrated"})
	}
}

// --- Archive contents ---

func (s *Server) archiveContentsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		archivePath := r.URL.Query().Get("path")
		if archivePath == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "path query parameter is required")
			return
		}
		entries, err := s.nfs.ListArchiveContents(name, archivePath)
		if err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, "not_found", "archive not found")
				return
			}
			s.log.Error("list archive contents failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list archive contents")
			return
		}
		if entries == nil {
			entries = []nfs.ArchiveEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
	}
}

// --- Write file ---

func (s *Server) writeFileHandler() http.HandlerFunc {
	type request struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		UID     int    `json:"uid"`
		GID     int    `json:"gid"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !validServerName(name) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid server name")
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		if req.Path == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "path is required")
			return
		}
		if err := s.nfs.WriteFile(name, req.Path, req.Content, req.UID, req.GID); err != nil {
			if errors.Is(err, nfs.ErrPathTraversal) {
				writeError(w, http.StatusBadRequest, "path_traversal", "path traversal detected")
				return
			}
			if strings.Contains(err.Error(), "exceeds maximum") {
				writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
				return
			}
			s.log.Error("write file failed", "error", err, "server", name, "trace_id", traceIDFromContext(r.Context()))
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to write file")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": req.Path})
	}
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

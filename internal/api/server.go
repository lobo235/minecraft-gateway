package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lobo235/minecraft-gateway/internal/nfs"
	"github.com/lobo235/minecraft-gateway/internal/rcon"
)

// Server holds the dependencies for the HTTP server.
type Server struct {
	nfs     nfs.Client
	rcon    rcon.Client
	apiKey  string
	log     *slog.Logger
	version string
}

// NewServer creates a Server wired with the given clients, API key, version string, and logger.
func NewServer(nfsClient nfs.Client, rconClient rcon.Client, apiKey, version string, log *slog.Logger) *Server {
	return &Server{
		nfs:     nfsClient,
		rcon:    rconClient,
		apiKey:  apiKey,
		log:     log,
		version: version,
	}
}

// Handler builds and returns the root http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	auth := bearerAuth(s.apiKey)

	// /health is unauthenticated — used by Nomad container health checks.
	mux.HandleFunc("GET /health", s.healthHandler())

	// Server management.
	mux.Handle("GET /servers", auth(http.HandlerFunc(s.listServersHandler())))
	mux.Handle("POST /servers", auth(http.HandlerFunc(s.createServerHandler())))
	mux.Handle("DELETE /servers/{name}", auth(http.HandlerFunc(s.deleteServerHandler())))

	// File download.
	mux.Handle("POST /servers/{name}/download", auth(http.HandlerFunc(s.downloadHandler())))
	mux.Handle("GET /servers/{name}/downloads/{downloadID}", auth(http.HandlerFunc(s.getDownloadStatusHandler())))

	// Archive contents.
	mux.Handle("GET /servers/{name}/archive-contents", auth(http.HandlerFunc(s.archiveContentsHandler())))

	// Disk usage.
	mux.Handle("GET /servers/{name}/disk-usage", auth(http.HandlerFunc(s.diskUsageHandler())))

	// File operations.
	mux.Handle("GET /servers/{name}/files", auth(http.HandlerFunc(s.listFilesHandler())))
	mux.Handle("GET /servers/{name}/files/read", auth(http.HandlerFunc(s.readFileHandler())))
	mux.Handle("GET /servers/{name}/files/grep", auth(http.HandlerFunc(s.grepFilesHandler())))
	mux.Handle("POST /servers/{name}/files/write", auth(http.HandlerFunc(s.writeFileHandler())))
	mux.Handle("POST /servers/{name}/files/move", auth(http.HandlerFunc(s.moveFileHandler())))
	mux.Handle("DELETE /servers/{name}/files/delete", auth(http.HandlerFunc(s.deleteFileHandler())))

	// Backup operations.
	mux.Handle("GET /servers/{name}/backups", auth(http.HandlerFunc(s.listBackupsHandler())))
	mux.Handle("POST /servers/{name}/backups", auth(http.HandlerFunc(s.startBackupHandler())))
	mux.Handle("GET /servers/{name}/backups/{backupID}", auth(http.HandlerFunc(s.getBackupStatusHandler())))

	// Restore and migrate.
	mux.Handle("POST /servers/{name}/restore", auth(http.HandlerFunc(s.restoreHandler())))
	mux.Handle("POST /servers/{name}/migrate", auth(http.HandlerFunc(s.migrateHandler())))

	// RCON operations.
	mux.Handle("POST /servers/{name}/rcon", auth(http.HandlerFunc(s.rconHandler())))
	mux.Handle("POST /servers/{name}/rcon/op", auth(http.HandlerFunc(s.rconOpHandler())))
	mux.Handle("POST /servers/{name}/rcon/deop", auth(http.HandlerFunc(s.rconDeopHandler())))
	mux.Handle("POST /servers/{name}/rcon/whitelist", auth(http.HandlerFunc(s.rconWhitelistHandler())))
	mux.Handle("GET /servers/{name}/rcon/players", auth(http.HandlerFunc(s.rconPlayersHandler())))

	return requestLogger(s.log)(mux)
}

// Run starts the HTTP server and blocks until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("server error: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.log.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

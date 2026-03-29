package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lobo235/minecraft-gateway/internal/rcon"
)

// Server holds the dependencies for the HTTP server.
type Server struct {
	rcon    rcon.Client
	apiKey  string
	log     *slog.Logger
	version string
}

// NewServer creates a Server wired with the given clients, API key, version string, and logger.
func NewServer(rconClient rcon.Client, apiKey, version string, log *slog.Logger) *Server {
	return &Server{
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

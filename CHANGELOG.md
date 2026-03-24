# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v1.0.4] - 2026-03-24

### Fixed
- RCON port discovery fetches full allocation details via `GetAllocation` instead of relying on list endpoint which omits port mappings
- `SafePath` resolves symlinks via `filepath.EvalSymlinks` to prevent symlink escape attacks
- `GrepFiles` uses 30-second timeout via `exec.CommandContext` to prevent DoS from pathological patterns
- Remove phantom `rcon/client_test.go` reference from CLAUDE.md

## [v1.0.2] - 2026-03-24

### Fixed
- RCON port discovery parses nested `AllocatedResources.Shared.Ports` from nomad-gateway allocation response — fixes "no running allocation with rcon port found" errors

### Changed
- Docker build workflow resolves version from git tags for non-tag builds

## [v1.0.1] - 2026-03-23

### Fixed

- Use HTTPS for gateway URLs in deploy spec to match Traefik TLS termination

## [v1.0.0] - 2026-03-23

### Added

- Initial project scaffold: go.mod, cmd/server/main.go, internal/ package layout
- Config loading from environment variables with validation
- HTTP server with Bearer token auth middleware and X-Trace-ID propagation
- NFS filesystem client: server CRUD, file operations, disk usage
- Backup system: async pzstd compression, status tracking, restore, migrate
- RCON client: resolves host/port from nomad-gateway, password from vault-gateway
- Upstream HTTP clients for nomad-gateway and vault-gateway
- All 20 API endpoints per spec
- Path traversal prevention with comprehensive unit tests
- Dockerfile with multi-stage build (includes pzstd runtime)
- Makefile, .golangci.yml, pre-commit hooks
- Nomad job spec and ACL policy (with placeholders)

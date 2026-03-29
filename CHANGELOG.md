# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Removed
- All filesystem operations (directory CRUD, file CRUD, downloads, archives, disk usage) — extracted to filesystem-gateway
- NFS client package
- Backup and restore operations — extracted to filesystem-gateway
- Download URL whitelist validation
- NFS_BASE_PATH, DATA_DIR, MAX_DOWNLOAD_SIZE, MAX_WRITE_FILE_SIZE, MAX_EXTRACT_SIZE config vars
- pzstd runtime dependency

### Changed
- Service now handles RCON operations only (5 routes + health)
- Simplified configuration: only gateway URLs/keys, API key, port, and log level
- Smaller Docker image (no zstd package)

## [v1.1.4] - 2026-03-26

### Fixed
- All directories and files created under the NFS base path are now chowned to the correct uid:gid via new `mkdirAllOwned` helper
- `runDownload` destination directories are chowned before file writes (fixes "permission denied" on modpack downloads)
- `WriteFile` parent directories are chowned to match the file's uid:gid
- `MoveFile` destination parent directories are chowned to uid:gid
- `Restore` now chowns all extracted files to uid:gid after extraction
- `StartBackup` backup directory and archive file are chowned to uid:gid

### Changed
- `StartBackup` API accepts optional `uid`/`gid` in request body
- `Restore` API accepts `uid`/`gid` in request body
- `MoveFile` accepts `uid`/`gid` parameters

## [v1.1.1] - 2026-03-24

### Fixed
- Replace external `tar` command with Go `archive/tar` stdlib — prevents symlink escape attacks in tar archives
- Add archive bomb protection: `MAX_EXTRACT_SIZE` env var (default 10GB) caps cumulative extracted bytes
- Block deletion of server root directory via delete endpoint
- Add `http.MaxBytesReader` on all JSON body handlers to prevent memory exhaustion
- Skip symlink entries in zip extraction for defense in depth
- Block HTTP redirects to private IP ranges (SSRF protection) during file downloads

### Added
- `POST /servers/{name}/files/move` — move/rename files within server filesystem
- `DELETE /servers/{name}/files/delete` — delete files/directories from server filesystem
- `MAX_EXTRACT_SIZE` env var (default 10GB)

## [v1.1.0] - 2026-03-24

### Added
- `POST /servers/{name}/download` — download files from CurseForge, Modrinth, FTB to server filesystem with optional extraction (zip, tar.gz, tar.zst)
- `GET /servers/{name}/archive-contents` — list files inside zip/tar archives on server filesystem
- `POST /servers/{name}/files/write` — write content to files on server filesystem
- Download modes: `overwrite` (default), `skip_existing`, `clean_first`
- URL allowlist: CurseForge CDN, Modrinth CDN, FTB domains
- `MAX_DOWNLOAD_SIZE` env var (default 2GB) and `MAX_WRITE_FILE_SIZE` env var (default 1MB)

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

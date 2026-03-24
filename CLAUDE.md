# minecraft-gateway

Authenticated HTTP API for Minecraft server filesystem management (NFS volume) and RCON command execution.
Part of the [homelab-ai](https://github.com/lobo235/homelab-ai) platform.

## Module

`github.com/lobo235/minecraft-gateway`

## Quick Start

```bash
cp .env.example .env
# Fill in required values
go run ./cmd/server
```

## Build, Test, Run

> Go is installed at `~/bin/go/bin/go` (also on `$PATH` via `.bashrc`).

```bash
# Build
make build

# Run tests
make test

# Run tests with verbose output
go test -v ./...

# Run linter
make lint

# Coverage report (opens in browser)
make cover

# Run the server (requires .env or env vars)
make run

# Build binary
go build -o minecraft-gateway ./cmd/server
```

## Project Layout

```
minecraft-gateway/
├── Dockerfile
├── Makefile
├── go.mod / go.sum
├── .env.example              # dev template — never commit real values
├── .gitignore
├── .golangci.yml             # strict linter config
├── .githooks/pre-commit      # runs lint + tests; activate with `make hooks`
├── CLAUDE.md                 # this file
├── README.md
├── CHANGELOG.md
├── cmd/
│   └── server/
│       └── main.go           # entry point
├── deploy/
│   ├── minecraft-gateway.hcl         # Nomad job spec (placeholders only)
│   └── minecraft-gateway.policy.hcl  # Nomad ACL policy
└── internal/
    ├── config/
    │   └── config.go         # ENV var loading & validation
    ├── nfs/
    │   ├── client.go         # NFS filesystem operations (path traversal prevention)
    │   └── client_test.go    # unit tests (path traversal, CRUD, backup/restore)
    ├── nomadgw/
    │   ├── client.go         # nomad-gateway HTTP client (allocation resolution)
    │   └── client_test.go
    ├── vaultgw/
    │   ├── client.go         # vault-gateway HTTP client (RCON password retrieval)
    │   └── client_test.go
    ├── rcon/
    │   └── client.go         # RCON client (resolves host/port + password from upstreams)
    └── api/
        ├── server.go         # HTTP mux + Run()
        ├── middleware.go     # Bearer auth + request logging + X-Trace-ID
        ├── handlers.go       # all route handlers
        ├── validate.go       # input validation (server names, player names)
        ├── errors.go         # writeError / writeJSON helpers
        └── health.go         # GET /health (unauthenticated)
```

## Configuration

All config via ENV vars. Loaded from `.env` in development (via `godotenv`; missing file silently ignored). In production, secrets are injected by Nomad Vault Workload Identity — the app never talks to Vault directly.

| Var | Required | Default | Purpose |
|-----|----------|---------|---------|
| `NFS_BASE_PATH` | yes | — | Base path for server data on NFS (e.g. `/mnt/data/minecraft`) |
| `NOMAD_GATEWAY_URL` | yes | — | Base URL of nomad-gateway (RCON allocation resolution) |
| `NOMAD_GATEWAY_KEY` | yes | — | API key for nomad-gateway |
| `VAULT_GATEWAY_URL` | yes | — | Base URL of vault-gateway (RCON password retrieval) |
| `VAULT_GATEWAY_KEY` | yes | — | API key for vault-gateway |
| `GATEWAY_API_KEY` | yes | — | Bearer token for callers of this API |
| `PORT` | no | `8080` | Listen port |
| `LOG_LEVEL` | no | `info` | Verbosity: `debug`, `info`, `warn`, `error` |
| `DATA_DIR` | no | `/data` | Directory for `.backup-status` tracking files |

## Architecture

```
cmd/server/main.go              — entry point, wires deps, handles SIGINT/SIGTERM
internal/config/config.go       — ENV-based config with validation
internal/api/server.go          — HTTP server, route registration
internal/api/middleware.go      — bearerAuth + requestLogger + X-Trace-ID propagation
internal/api/handlers.go        — route handlers (servers, files, backups, RCON)
internal/api/validate.go        — input validation (server names, player names)
internal/api/errors.go          — writeError / writeJSON helpers
internal/api/health.go          — GET /health handler (unauthenticated)
internal/nfs/client.go          — NFS filesystem client (path traversal prevention, CRUD, backups)
internal/nomadgw/client.go      — nomad-gateway HTTP client (allocation resolution for RCON)
internal/vaultgw/client.go      — vault-gateway HTTP client (RCON password retrieval)
internal/rcon/client.go         — RCON client (resolves connection from upstreams, uses gorcon/rcon)
```

**RCON flow:** Caller sends `POST /servers/{name}/rcon` with `{"command":"..."}`. The handler calls the RCON client, which:
1. Calls nomad-gateway `GET /jobs/{name}/allocations` to find the running allocation's RCON port
2. Calls vault-gateway `GET /secrets/minecraft/{name}` to retrieve the RCON password
3. Connects via gorcon/rcon and executes the command

**Backup flow:** `POST /servers/{name}/backups` triggers an async backup using pzstd (parallel zstd). Returns immediately with a backup ID. Status tracked in `.backup-status` JSON files at `DATA_DIR/<server-name>.backup-status`. Files stored at `<NFS_BASE_PATH>/<server>/backups/<id>.tar.zst`.

**Restore sequencing:** `POST /servers/{name}/restore` performs NO liveness check — it always attempts the restore. The MCP server orchestration layer is responsible for stopping the server before restore and restarting it afterward. If restore runs while the server is live, the JVM will overwrite restored data on the next autosave, causing silent data loss.

## API Routes

All routes except `/health` require `Authorization: Bearer <GATEWAY_API_KEY>`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | No | Returns `{"status":"ok","version":"..."}` |
| GET | `/servers` | Yes | List server directories on NFS volume |
| POST | `/servers` | Yes | Create server dir `{"name":"...","uid":N,"gid":N}` |
| DELETE | `/servers/{name}` | Yes | Delete server dir (requires `?confirm=true`) |
| GET | `/servers/{name}/disk-usage` | Yes | Disk usage in bytes |
| GET | `/servers/{name}/files` | Yes | List files (`?path=subdir`) |
| GET | `/servers/{name}/files/read` | Yes | Read file (`?path=logs/latest.log`) max 1MB |
| GET | `/servers/{name}/files/grep` | Yes | Grep (`?path=...&pattern=...`) max 10k lines/5MB |
| GET | `/servers/{name}/backups` | Yes | List available `.tar.zst` backups |
| POST | `/servers/{name}/backups` | Yes | Trigger async backup; returns backup ID |
| GET | `/servers/{name}/backups/{backupID}` | Yes | Backup status/details |
| POST | `/servers/{name}/restore` | Yes | Restore from backup `{"backup_id":"..."}` |
| POST | `/servers/{name}/migrate` | Yes | Rename server `{"new_name":"..."}` |
| POST | `/servers/{name}/rcon` | Yes | Send RCON command `{"command":"..."}` |
| POST | `/servers/{name}/rcon/op` | Yes | Op player `{"player":"..."}` |
| POST | `/servers/{name}/rcon/deop` | Yes | Deop player `{"player":"..."}` |
| POST | `/servers/{name}/rcon/whitelist` | Yes | Whitelist `{"action":"add\|remove","player":"..."}` |
| GET | `/servers/{name}/rcon/players` | Yes | Online player list via RCON `list` command |

### Input Validation

- **Server names:** `^[a-z0-9][a-z0-9-]{0,47}$`
- **Player names:** `^[a-zA-Z0-9_]{1,16}$`

### Path Traversal Prevention

All path parameters are resolved via `filepath.Abs()` and verified to have `NFS_BASE_PATH` as a prefix before any filesystem operation. Requests with `../` sequences, absolute paths outside the base, or URL-encoded traversal sequences are rejected with HTTP 400 and code `path_traversal`.

## Testing Approach

Tests use `httptest.NewServer` to mock upstream HTTP APIs (nomad-gateway, vault-gateway) — no live dependencies required.

```
internal/nfs/client_test.go     — path traversal prevention, filesystem CRUD, backup operations
internal/nomadgw/client_test.go — upstream client unit tests
internal/vaultgw/client_test.go — upstream client unit tests
internal/api/server_test.go     — handler tests via httptest (all endpoints)
internal/config/config_test.go  — config loading and validation
```

Key patterns:
- Each test registers a mock endpoint, calls the client/handler, asserts return value and that mock was hit
- Table-driven tests for input validation (server names, player names, path traversal)
- Test both success paths and error paths (upstream 4xx, 5xx, network error)
- **Path traversal tests are mandatory:** `../`, `../../`, absolute paths, URL-encoded `%2e%2e%2f`

## Coding Conventions

- No external router, ORM, or framework — minimal dependency footprint
- Error responses always use `writeError(w, status, code, message)` with machine-readable `code`
- Route handlers return `http.HandlerFunc`
- All upstream errors wrapped with `fmt.Errorf("context: %w", err)`
- `X-Trace-ID` header propagated from request context to all upstream calls and log lines
- Structured JSON logging via `log/slog`; version logged on startup; every request access-logged
- Never log RCON passwords or secret values

## Security Rules

> **Claude must enforce all rules below on every commit and push without exception.**

1. **Never commit secrets:** No `.env`, tokens, API keys, passwords, or credentials of any kind.
2. **Never commit infrastructure identifiers:** No real hostnames, IP addresses, datacenter names, node pool names, Consul service names, Vault paths with real values, Traefik routing rules with real domains, or any value that reveals homelab architecture. Use generic placeholders (`dc1`, `default`, `example.com`, `your-node-pool`, `your-service`).
3. **Unknown files:** If `git status` shows a file Claude didn't create, ask the operator before staging it.
4. **Pre-commit checks (must all pass before committing):**
   - `go test ./...` — all tests must pass
   - `golangci-lint run` — no lint errors
5. **Docs accuracy:** Review all changed `.md` files before committing — documentation must reflect the current state of the code in the same commit.
6. **Version bump:** Before any `git commit`, review the changes and determine the appropriate SemVer bump (MAJOR/MINOR/PATCH). Present the rationale and proposed new version to the operator and wait for confirmation before tagging or referencing the new version.
7. **Push confirmation:** Before any `git push`, show the operator a summary of what will be pushed (commits, branch, remote) and wait for explicit confirmation.
8. **Commit messages:** Must not contain real hostnames, IPs, or infrastructure identifiers.
9. **Never log RCON passwords or secret values** in any log line.

## Versioning & Releases

SemVer (`MAJOR.MINOR.PATCH`). Git tags are the source of truth.

```bash
git tag v1.2.3 && git push origin v1.2.3
```

This triggers the Docker workflow which publishes:
- `ghcr.io/lobo235/minecraft-gateway:v1.2.3`
- `ghcr.io/lobo235/minecraft-gateway:v1.2`
- `ghcr.io/lobo235/minecraft-gateway:latest`
- `ghcr.io/lobo235/minecraft-gateway:<short-sha>`

Version is embedded at build time: `-ldflags "-X main.version=v1.2.3"` — defaults to `"dev"` for local builds. Exposed in `GET /health` response and logged on startup.

## Docker

```bash
# Build (version defaults to "dev")
docker build -t minecraft-gateway .

# Build with explicit version
docker build --build-arg VERSION=v1.2.3 -t minecraft-gateway .

# Run
docker run --env-file .env -p 8080:8080 minecraft-gateway
```

Multi-stage build: `golang:1.24-alpine` → `alpine:3.21`. Statically compiled (`CGO_ENABLED=0`). Runtime image includes `zstd` package for pzstd backup compression.

## Known Limitations

- **Restore requires server stop:** `POST /servers/{name}/restore` does not check server liveness. If the Minecraft server is running during restore, the JVM will overwrite restored world data on the next autosave, causing silent data loss. The MCP server must stop the Nomad job before calling restore.
- **Single backup status per server:** Only the most recent backup status is tracked per server in the `.backup-status` file. Starting a new backup overwrites the previous status.
- **Backup timestamp IDs:** Backup IDs use second-precision UTC timestamps (`2006-01-02T15-04-05`). Starting two backups for the same server within the same second will collide.
- **pzstd required at runtime:** The `pzstd` binary must be available in `PATH` for backup/restore operations. The Dockerfile includes it via `apk add zstd`.

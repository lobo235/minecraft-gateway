# minecraft-gateway

Authenticated HTTP API for Minecraft server RCON command execution.
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
        ├── handlers.go       # RCON route handlers
        ├── validate.go       # input validation (server names, player names)
        ├── errors.go         # writeError / writeJSON helpers
        └── health.go         # GET /health (unauthenticated)
```

## Configuration

All config via ENV vars. Loaded from `.env` in development (via `godotenv`; missing file silently ignored). In production, secrets are injected by Nomad Vault Workload Identity — the app never talks to Vault directly.

| Var | Required | Default | Purpose |
|-----|----------|---------|---------|
| `NOMAD_GATEWAY_URL` | yes | — | Base URL of nomad-gateway (RCON allocation resolution) |
| `NOMAD_GATEWAY_KEY` | yes | — | API key for nomad-gateway |
| `VAULT_GATEWAY_URL` | yes | — | Base URL of vault-gateway (RCON password retrieval) |
| `VAULT_GATEWAY_KEY` | yes | — | API key for vault-gateway |
| `GATEWAY_API_KEY` | yes | — | Bearer token for callers of this API |
| `PORT` | no | `8080` | Listen port |
| `LOG_LEVEL` | no | `info` | Verbosity: `debug`, `info`, `warn`, `error` |

## Architecture

```
cmd/server/main.go              — entry point, wires deps, handles SIGINT/SIGTERM
internal/config/config.go       — ENV-based config with validation
internal/api/server.go          — HTTP server, route registration
internal/api/middleware.go      — bearerAuth + requestLogger + X-Trace-ID propagation
internal/api/handlers.go        — RCON route handlers
internal/api/validate.go        — input validation (server names, player names)
internal/api/errors.go          — writeError / writeJSON helpers
internal/api/health.go          — GET /health handler (unauthenticated)
internal/nomadgw/client.go      — nomad-gateway HTTP client (allocation resolution for RCON)
internal/vaultgw/client.go      — vault-gateway HTTP client (RCON password retrieval)
internal/rcon/client.go         — RCON client (resolves connection from upstreams, uses gorcon/rcon)
```

**RCON flow:** Caller sends `POST /servers/{name}/rcon` with `{"command":"..."}`. The handler calls the RCON client, which:
1. Calls nomad-gateway `GET /jobs/{name}/allocations` to find the running allocation's RCON port
2. Calls vault-gateway `GET /secrets/minecraft/{name}` to retrieve the RCON password
3. Connects via gorcon/rcon and executes the command

## API Routes

All routes except `/health` require `Authorization: Bearer <GATEWAY_API_KEY>`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | No | Returns `{"status":"ok","version":"..."}` |
| POST | `/servers/{name}/rcon` | Yes | Send RCON command `{"command":"..."}` |
| POST | `/servers/{name}/rcon/op` | Yes | Op player `{"player":"..."}` |
| POST | `/servers/{name}/rcon/deop` | Yes | Deop player `{"player":"..."}` |
| POST | `/servers/{name}/rcon/whitelist` | Yes | Whitelist `{"action":"add\|remove","player":"..."}` |
| GET | `/servers/{name}/rcon/players` | Yes | Online player list via RCON `list` command |

### Input Validation

- **Server names:** `^[a-z0-9][a-z0-9-]{0,47}$`
- **Player names:** `^[a-zA-Z0-9_]{1,16}$`

## Testing Approach

Tests use `httptest.NewServer` to mock upstream HTTP APIs (nomad-gateway, vault-gateway) — no live dependencies required.

```
internal/nomadgw/client_test.go — upstream client unit tests
internal/vaultgw/client_test.go — upstream client unit tests
internal/api/server_test.go     — handler tests via httptest (all RCON endpoints)
internal/config/config_test.go  — config loading and validation
```

Key patterns:
- Each test registers a mock endpoint, calls the client/handler, asserts return value and that mock was hit
- Table-driven tests for input validation (server names, player names)
- Test both success paths and error paths (upstream 4xx, 5xx, network error)

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

Multi-stage build: `golang:1.24-alpine` -> `alpine:3.21`. Statically compiled (`CGO_ENABLED=0`). Minimal runtime image with only `ca-certificates`.

## Known Limitations

- **RCON requires running server:** RCON commands fail with 404 if no running Nomad allocation is found for the target server.
- **Single RCON connection per request:** Each RCON command opens a new connection. There is no connection pooling.

# minecraft-gateway

Authenticated HTTP API for Minecraft server RCON command execution.
Part of the [homelab-ai](https://github.com/lobo235/homelab-ai) platform.

## Features

- **RCON commands:** Execute arbitrary commands, op/deop players, whitelist management, player list
- **Security:** Bearer token authentication, X-Trace-ID correlation
- **Upstream resolution:** Automatically resolves RCON host/port from nomad-gateway and password from vault-gateway

## Quick Start

```bash
cp .env.example .env
# Fill in required values
go run ./cmd/server
```

## Build & Run

```bash
make build    # Build binary
make test     # Run tests
make lint     # Run linter
make cover    # Coverage report
make run      # Run locally
make hooks    # Install pre-commit hooks
```

## Docker

```bash
docker build -t minecraft-gateway .
docker run --env-file .env -p 8080:8080 minecraft-gateway
```

## API

All endpoints except `/health` require `Authorization: Bearer <GATEWAY_API_KEY>`.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Health check (unauthenticated) |
| POST | `/servers/{name}/rcon` | Send RCON command |
| POST | `/servers/{name}/rcon/op` | Op player |
| POST | `/servers/{name}/rcon/deop` | Deop player |
| POST | `/servers/{name}/rcon/whitelist` | Whitelist add/remove |
| GET | `/servers/{name}/rcon/players` | Online player list |

## Configuration

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `NOMAD_GATEWAY_URL` | Yes | — | nomad-gateway base URL |
| `NOMAD_GATEWAY_KEY` | Yes | — | nomad-gateway API key |
| `VAULT_GATEWAY_URL` | Yes | — | vault-gateway base URL |
| `VAULT_GATEWAY_KEY` | Yes | — | vault-gateway API key |
| `GATEWAY_API_KEY` | Yes | — | Bearer token for callers |
| `PORT` | No | `8080` | Listen port |
| `LOG_LEVEL` | No | `info` | Log verbosity |

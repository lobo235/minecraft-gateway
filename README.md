# minecraft-gateway

Authenticated HTTP API for Minecraft server filesystem management (NFS volume) and RCON command execution.
Part of the [homelab-ai](https://github.com/lobo235/homelab-ai) platform.

## Features

- **Server management:** Create, list, delete server directories on the NFS volume
- **File operations:** List files, read files (1MB max), grep with pattern matching
- **Backup system:** Async backups via pzstd compression, restore, status tracking
- **RCON commands:** Execute arbitrary commands, op/deop players, whitelist management, player list
- **Security:** Path traversal prevention, Bearer token authentication, X-Trace-ID correlation

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
| GET | `/servers` | List server directories |
| POST | `/servers` | Create server directory |
| DELETE | `/servers/{name}` | Delete server directory |
| GET | `/servers/{name}/disk-usage` | Disk usage |
| GET | `/servers/{name}/files` | List files |
| GET | `/servers/{name}/files/read` | Read file |
| GET | `/servers/{name}/files/grep` | Grep files |
| GET | `/servers/{name}/backups` | List backups |
| POST | `/servers/{name}/backups` | Start backup (async) |
| GET | `/servers/{name}/backups/{id}` | Backup status |
| POST | `/servers/{name}/restore` | Restore from backup |
| POST | `/servers/{name}/migrate` | Rename server |
| POST | `/servers/{name}/rcon` | Send RCON command |
| POST | `/servers/{name}/rcon/op` | Op player |
| POST | `/servers/{name}/rcon/deop` | Deop player |
| POST | `/servers/{name}/rcon/whitelist` | Whitelist add/remove |
| GET | `/servers/{name}/rcon/players` | Online player list |

## Configuration

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `NFS_BASE_PATH` | Yes | — | NFS volume base path |
| `NOMAD_GATEWAY_URL` | Yes | — | nomad-gateway base URL |
| `NOMAD_GATEWAY_KEY` | Yes | — | nomad-gateway API key |
| `VAULT_GATEWAY_URL` | Yes | — | vault-gateway base URL |
| `VAULT_GATEWAY_KEY` | Yes | — | vault-gateway API key |
| `GATEWAY_API_KEY` | Yes | — | Bearer token for callers |
| `PORT` | No | `8080` | Listen port |
| `LOG_LEVEL` | No | `info` | Log verbosity |
| `DATA_DIR` | No | `/data` | Backup status file directory |

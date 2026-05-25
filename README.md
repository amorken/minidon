# minidon

> A Mastodon public-timeline streaming web app: live feed, full-text search,
> single-binary deployment.

**Status**: early scaffolding — source files are stubs; no business logic implemented yet.

---

## Overview

`minidon` connects to a Mastodon instance's public streaming API, maintains a
bounded in-memory ring buffer of the most-recent statuses, fans them into a
MeiliSearch index for full-text search, and serves a React/TypeScript single-page
application — all from one statically-linked Go binary with embedded frontend assets.

See [`docs/architecture.md`](docs/architecture.md) for the full design.

---

## Prerequisites

| Tool | Minimum version |
|------|----------------|
| Go   | 1.22            |
| Node | 20              |
| Docker & Compose | optional, for containerised deployment |

---

## Quick Start

### Local (binary)

```sh
# 1. Build the frontend
make web

# 2. Build the Go binary (embeds web/dist)
make build

# 3. Run
MINIDON_MASTODON_INSTANCE=mastodon.social ./bin/minidon
```

Open <http://localhost:8080> in your browser.

### Docker Compose

```sh
docker compose -f deploy/docker-compose.yml up
```

This starts `minidon` and a `meilisearch` container with a shared named volume.

---

## Configuration

All settings are read from environment variables (12-factor).

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIDON_LISTEN` | `:8080` | TCP address to listen on |
| `MINIDON_MASTODON_INSTANCE` | *(required)* | Mastodon instance hostname, e.g. `mastodon.social` |
| `MINIDON_MEILI_URL` | `http://localhost:7700` | MeiliSearch base URL |
| `MINIDON_MEILI_KEY` | `""` | MeiliSearch API key (leave empty for no-auth dev) |
| `MINIDON_BUFFER_SIZE` | `500` | Number of recent statuses kept in the ring buffer |

---

## Commands

The `minidon` binary supports the following subcommands:

- `web` (default): Runs the web application server.
- `cli`: Runs the streaming timeline client CLI to print statuses to `stdout`.
- `delete-index`: Clears all indexed statuses and Mastodon pagination state from MeiliSearch. Requires MeiliSearch to be configured and reachable.

---

## Repository Layout

```
minidon/
├── cmd/minidon/        Binary entrypoint
├── internal/
│   ├── config/         Env/flag config loading
│   ├── mastodon/       Streaming client (go-mastodon behind an interface)
│   ├── ingest/         Fan-out pipeline: stream → buffer + index + SSE
│   ├── buffer/         In-memory ring buffer
│   ├── index/          MeiliSearch adapter
│   ├── api/            HTTP handlers (timeline, search, SSE, health)
│   ├── static/         go:embed wrapper for web/dist
│   └── model/          Shared status DTO
├── web/                React + Vite + TypeScript SPA
├── deploy/             Dockerfile, docker-compose files
├── scripts/            dev.sh, build.sh helpers
├── docs/               Architecture and API reference
└── Makefile
```

---

## Development Workflow

```sh
# Terminal 1 — Vite dev server with HMR (http://localhost:5173)
make dev-web

# Terminal 2 — Go server with proxy to Vite
make dev-go
```

Or use the convenience script which starts both concurrently:

```sh
./scripts/dev.sh
```

The Vite config proxies `/api/*` to the Go server running on `:8080`.

---

## Building for Production

```sh
make web    # produces web/dist/
make build  # produces bin/minidon (static binary with embedded assets)
```

The binary has no runtime dependencies — no Node, no static file server needed.

---

## Docker / Compose

```sh
# Production stack
docker compose -f deploy/docker-compose.yml up --build

# Development overrides (live reload, bind mounts)
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml up
```

---

## License

Apache 2.0 — see [LICENSE](LICENSE).

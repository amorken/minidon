# minidon

> A Mastodon public-timeline streaming web app: live feed, full-text search,
> single-binary deployment.

**Status**: Fully implemented demo and experiment playground for streaming Mastodon timelines with Go, React/TypeScript, and MeiliSearch.

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
| Go   | 1.26            |
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
MINIDON_MASTODON_INSTANCE=https://mstdn.social ./bin/minidon
```

Open <http://localhost:8080> in your browser.

### Docker Compose

```sh
docker compose -f deploy/docker-compose.yml up
```

This starts `minidon` and a `meilisearch` container with a shared named volume.

---

## Configuration

All settings can be configured via command-line flags or environment variables (12-factor), parsed using the Kong library. 

An example dotenv template is provided in [.env.example](file:///home/anders/git/minidon/.env.example). You can copy this template to set your environment variables:

```sh
cp .env.example .env
# Edit .env with your credentials
```

You can export these variables into your shell or run the application by prefixing command execution (e.g., `export $(cat .env | xargs) && ./bin/minidon`).

The application supports two subcommands:
* `web`: Run the web application server (default, if no command is specified).
* `cli`: Run the streaming timeline client CLI.

### Global Options

| Command Line Flag | Environment Variable | Default | Description |
|-------------------|----------------------|---------|-------------|
| `--disable-search` | `MINIDON_DISABLE_SEARCH` | `false` | Disable search functionality / MeiliSearch connection |
| `--listen` | `MINIDON_LISTEN` | `:8080` | TCP listen address to listen on |
| `--mastodon-instance` | `MINIDON_MASTODON_INSTANCE` | *(required)* | Mastodon instance hostname, e.g., `mastodon.social` |
| `--mastodon-client-id` | `MINIDON_MASTODON_CLIENT_ID` | `""` | Mastodon client ID |
| `--mastodon-client-secret` | `MINIDON_MASTODON_CLIENT_SECRET` | `""` | Mastodon client secret |
| `--mastodon-access-token` | `MINIDON_MASTODON_ACCESS_TOKEN` | *(required)* | Mastodon access token |
| `--mastodon-stream-path` | `MINIDON_MASTODON_STREAM_PATH` | `api/v1/streaming` | Mastodon streaming API path |
| `--mastodon-stream` | `MINIDON_MASTODON_STREAM` | `public` | Mastodon stream type: `user`, `public`, `user:local`, or `public:local` |
| `--meili-url` | `MINIDON_MEILI_URL` | `http://localhost:7700` | MeiliSearch base URL |
| `--meili-key` | `MINIDON_MEILI_KEY` | `""` | MeiliSearch API key (supports Default Admin Key resolution using master key) |
| `--buffer-size` | `MINIDON_BUFFER_SIZE` | `500` | Ring buffer capacity |
| `-v, --verbose` | `MINIDON_VERBOSE` | `false` | Enable verbose logging |

### `cli` Command Options

| Command Line Flag | Default | Description |
|-------------------|---------|-------------|
| `--format` | `json` | Output format for cli mode: `json` or `text` |

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

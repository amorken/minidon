# minidon — Architecture

---

## 1. Goals & Non-Goals

### Goals

- Stream a Mastodon instance's public timeline in real time.
- Maintain a bounded in-memory buffer of recent statuses for instant reads.
- Provide full-text search over historical statuses via MeiliSearch.
- Expose a TypeScript SPA served directly from the Go binary (single-binary deployment).
- Simple configuration via environment variables (12-factor).

### Non-Goals (for now)

- Posting, replying, or any write operations to Mastodon.
- Authentication or multi-user support.
- Federation or ActivityPub protocol handling.
- Supporting multiple Mastodon instances simultaneously.

---

## 2. High-Level Diagram

```
Mastodon instance
  │
  │  WebSocket streaming API
  │  (wss://<instance>/api/v1/streaming?stream=public)
  ▼
┌─────────────────────────────────────────────────────┐
│  internal/mastodon  — Client interface + impl        │
│  (mattn/go-mastodon, reconnect loop)                 │
└───────────────────┬─────────────────────────────────┘
                    │  chan *model.Status
                    ▼
┌─────────────────────────────────────────────────────┐
│  internal/ingest  — fan-out pipeline                 │
└──────┬──────────────────┬──────────────────┬────────┘
       │                  │                  │
       ▼                  ▼                  ▼
┌────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  buffer    │  │  index (Meili)  │  │  SSE subscribers │
│  ring buf  │  │  batched writer │  │  (api/stream)    │
└────────────┘  └─────────────────┘  └─────────────────┘
       ▲                  ▲
       │                  │
┌─────────────────────────────────────────────────────┐
│  internal/api  — HTTP handlers                       │
│  GET /api/timeline   GET /api/search   GET /api/stream│
│  GET /healthz        GET /readyz                     │
│  GET /*  → embedded SPA (internal/static)            │
└─────────────────────────────────────────────────────┘
       ▲
       │  HTTP
  Browser SPA (web/dist, embedded in binary)
```

---

## 3. Component Descriptions

### Mastodon Client (`internal/mastodon`)

Connects to `wss://<instance>/api/v1/streaming?stream=public`.

- Uses `mattn/go-mastodon` as the WebSocket wrapper behind a `Client` interface,
  so it can be replaced with a hand-rolled REST/WS implementation if needed.
- On disconnect: reconnects with exponential back-off (jitter, configurable caps).
- Periodic fallback: `GET /api/v1/timelines/public` for backfill after reconnect
  to avoid gaps in the buffer.
- Emits `*model.Status` values on a read-only channel for the ingest pipeline.

### Ingest Pipeline (`internal/ingest`)

Single goroutine that fans out from one source channel to three consumers:

1. **Ring buffer** — synchronous write (fast, bounded).
2. **MeiliSearch batch writer** — debounced: flush every 1 s _or_ every 100
   documents, whichever comes first.
3. **SSE fan-out** — broadcasts to all currently-registered `http.ResponseWriter`
   SSE clients; slow clients are dropped after a configurable timeout.

`Subscribe` / `Unsubscribe` methods allow the HTTP stream handler to register and
deregister clients safely (uses an internal mutex or channel-based actor pattern).

### Ring Buffer (`internal/buffer`)

Bounded, in-memory, thread-safe slice of `*model.Status`.

- Default capacity: 500 items; configurable via `MINIDON_BUFFER_SIZE`.
- Eviction: oldest entry dropped when capacity is exceeded.
- `Recent(n int)` returns the n most-recent statuses in reverse chronological order.
- Write locking: `sync.Mutex` protects writes; reads use an `atomic.Pointer` swap
  of an immutable snapshot for lock-free concurrent reads.

### Index (`internal/index`)

Interface:

```go
type Index interface {
    Index(statuses []model.Status) error
    Search(query string, opts SearchOptions) (SearchResult, error)
}
```

**MeiliSearch implementation** (`meili.go`):

- Primary index name: `statuses`.
- Searchable attributes: `content`, `account.acct`, `account.display_name`, `tags.name`.
- Sortable: `created_at`.
- Filterable: `language`, `tags.name`.
- `EnsureSettings()` applies the above configuration idempotently on startup.

### HTTP API (`internal/api`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/timeline?limit=N` | Most-recent N statuses from the ring buffer (default 50, max 200) |
| GET | `/api/search?q=&limit=&offset=` | Full-text search via MeiliSearch |
| GET | `/api/stream` | SSE stream |
| GET | `/healthz` | Liveness probe — always 200 OK |
| GET | `/readyz` | Readiness probe — 200 OK (checks Mastodon connection status) |

Routes are registered using Go 1.22 `http.ServeMux` method+pattern matching
(e.g. `mux.HandleFunc("GET /api/timeline", ...)`). The SPA handler is mounted
on `GET /` with lower priority than specific patterns.

### Static Assets (`internal/static`)

The `//go:embed web/dist` directive lives in `embed.go` at the module root (because
`go:embed` paths are relative to the source file directory, it must be at or above
the module root — `cmd/minidon/main.go` is too deep). The embedded `embed.FS` is
passed to `static.NewHandler(fsys)`.

`NewHandler` creates a sub-filesystem rooted at `web/dist` and returns an
`http.Handler` that:

1. Rejects non-GET/HEAD requests with 405.
2. Returns 404 for any path starting with `/api/` (no SPA fallback for API routes).
3. Attempts to serve the requested file from the embedded FS.
4. If the file does not exist (or is a directory), serves `index.html` as the
   SPA fallback for client-side routing.

**Cache-Control headers:**

| Path | Cache-Control |
|------|---------------|
| `/index.html` | `no-cache` |
| `/assets/*` (hashed filenames) | `public, max-age=31536000, immutable` |
| Everything else | `public, max-age=300` |

---

## 4. Data Model

`model.Status` is a subset of the Mastodon Status entity.

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Mastodon status ID (snowflake) |
| `uri` | string | ActivityPub URI |
| `url` | string | Web URL |
| `created_at` | time.Time | RFC 3339 |
| `content` | string | Sanitised HTML (or strip to plain text — TBD) |
| `language` | string | BCP 47 language tag |
| `account.acct` | string | `user@instance` |
| `account.display_name` | string | |
| `account.avatar` | string | URL |
| `media_attachments[].preview_url` | string | |
| `media_attachments[].type` | string | `image`, `video`, `gifv`, `audio`, `unknown` |
| `tags[].name` | string | Hashtag (without `#`) |
| `reblog` | *Status | Recursively embedded, depth 1 only |

---

## 5. Frontend

- **Framework**: React 18 + Vite 5 + TypeScript 5.
- **State**: React hooks; a lightweight store (Zustand or plain context) for global
  state (live timeline, search results).
- **Live updates**: `EventSource` connecting to `GET /api/stream`.
- **Initial views**:
  - **Live timeline** — scrolling feed of statuses, updated via SSE.
  - **Search** — text input, debounced, queries `/api/search`; result list.
  - **Status detail** — modal or expand-in-place showing full content and media.
- **API client**: typed fetch wrappers in `web/src/api/`.

> Preact is a viable drop-in alternative if bundle size becomes a concern.

---

## 6. Build & Deployment

### Local Build

```sh
make web    # Node 20: npm ci && npm run build → web/dist/
make build  # Go 1.22: go build → bin/minidon (embeds web/dist)
```

The resulting binary is self-contained: `go:embed` bundles `web/dist` into the
executable at compile time. Running `./bin/minidon` (or `./bin/minidon web`) starts an HTTP server on
`:8080` (configurable via `--listen` or `MINIDON_LISTEN`) that serves the SPA and API routes. Alternatively, running `./bin/minidon cli` runs the streaming client CLI.

### Docker (Multi-Stage)

```
Stage 1: node:20-alpine   — npm ci && npm run build
Stage 2: golang:1.22-alpine — go build (copies web/dist from stage 1)
Stage 3: distroless/static-debian12:nonroot — final image, binary only
```

The final image is ~10–20 MB with no shell, no package manager, no Node runtime.

### Compose

```sh
docker compose -f deploy/docker-compose.yml up --build
```

Brings up `minidon` (port 8080) + `meilisearch` (port 7700, internal only) with a
named volume for Meili data and an isolated bridge network.

---

## 7. Configuration

All settings can be configured via command-line flags or environment variables, parsed using the [Kong](https://github.com/alecthomas/kong) library.

The application supports three subcommands:
* `web`: Run the web application server (default, if no command is specified).
* `cli`: Run the streaming timeline client CLI.
* `delete-index`: Delete/clear out index state from MeiliSearch (both statuses and pagination state).


### Global Options

| Command Line Flag | Environment Variable | Default | Description |
|-------------------|----------------------|---------|-------------|
| `--disable-search` | `MINIDON_DISABLE_SEARCH` | `false` | Disable search functionality / MeiliSearch connection |
| `--listen` | `MINIDON_LISTEN` | `:8080` | TCP listen address to listen on |
| `--mastodon-instance` | `MINIDON_MASTODON_INSTANCE` | *(required)* | Mastodon instance hostname |
| `--mastodon-client-id` | `MINIDON_MASTODON_CLIENT_ID` | `""` | Mastodon client ID |
| `--mastodon-client-secret` | `MINIDON_MASTODON_CLIENT_SECRET` | `""` | Mastodon client secret |
| `--mastodon-access-token` | `MINIDON_MASTODON_ACCESS_TOKEN` | *(required)* | Mastodon access token |
| `--mastodon-stream-path` | `MINIDON_MASTODON_STREAM_PATH` | `api/v1/streaming` | Mastodon streaming API path |
| `--mastodon-stream` | `MINIDON_MASTODON_STREAM` | `public` | Mastodon stream type: `user`, `public`, `user:local`, or `public:local` |
| `--meili-url` | `MINIDON_MEILI_URL` | `http://localhost:7700` | MeiliSearch base URL |
| `--meili-key` | `MINIDON_MEILI_KEY` | `""` | MeiliSearch API key |
| `--buffer-size` | `MINIDON_BUFFER_SIZE` | `500` | Ring buffer capacity |
| `-v, --verbose` | `MINIDON_VERBOSE` | `false` | Enable verbose logging |

### `cli` Command Options

| Command Line Flag | Default | Description |
|-------------------|---------|-------------|
| `--format` | `json` | Output format for cli mode: `json` or `text` |

---

## 8. Observability

- **Structured logging**: `log/slog` with JSON output in production, text in dev.
- **Health endpoints**: `/healthz` (liveness) and `/readyz` (readiness).
- **Metrics**: `/metrics` Prometheus endpoint reserved for a future pass.

---

## 9. Open Questions & Tradeoffs

### Streaming transport
- `mattn/go-mastodon` WebSocket wrapper vs. a hand-rolled `gorilla/websocket` client.
  The mattn library simplifies auth and event parsing but adds a dependency; a raw
  client would have full control over reconnect semantics.

### Buffer deduplication on reconnect
- After reconnect, the REST backfill may return statuses already in the buffer.
  Options: (a) skip on duplicate ID (requires ID lookup — O(1) with a map),
  (b) accept duplicates and let clients dedup. Map approach preferred.

### Streaming transport for browser clients

Three options were considered for streaming statuses from the Go server to the
browser SPA:

| Aspect | SSE over REST | WebSocket | gRPC / gRPC-Web |
|--------|--------------|-----------|-----------------|
| Direction | Server → client (unidirectional) | Bidirectional | Server → client (server-streaming) |
| Browser API | Native `EventSource` | Native `WebSocket` | Requires gRPC-Web + Envoy proxy |
| Auto-reconnect | Built into `EventSource` | Manual | Manual or via library |
| HTTP version | 1.1+ | 1.1+ | Requires HTTP/2 (proxy for browsers) |
| Infrastructure | None | None | Envoy sidecar/proxy required |
| Typing | JSON (manual) | JSON (manual) | Protobuf (code-generated) |
| Debugging | Plain text in browser DevTools | Binary frames | Binary protobuf frames |

**Rationale**: The stream is strictly server → client (no need for bidirectional
communication). SSE over REST is the simplest option: it uses a standard HTTP GET
with `Content-Type: text/event-stream`, works through HTTP/1.1 proxies and CDNs,
provides native browser support via `EventSource` with built-in auto-reconnection,
and requires no additional infrastructure. WebSocket adds unnecessary complexity
for a unidirectional stream. gRPC-Web would provide strong typing and code
generation but requires an Envoy proxy layer between browser and server, adding
operational overhead that isn't justified for a single streaming endpoint.

**Decision: SSE over REST** — `GET /api/stream` with `text/event-stream`,
consumed via the browser `EventSource` API.

### Search backend comparison

| Backend | Pros | Cons |
|---------|------|------|
| **MeiliSearch** | Managed relevance, instant search, typo-tolerance | External dependency |
| Bleve (embedded) | No external service | Less feature-rich, larger binary |
| Typesense | Similar to Meili, good SDKs | Less mature Go SDK |
| SQLite FTS5 | Zero external deps, embedded | Basic relevance, no typo-tolerance |

MeiliSearch chosen for first pass; interface design allows substitution.

---

## 10. Future Work

- Multiple Mastodon instance support (fan-in from several streams).
- Authentication (OAuth2 flow for user timelines or administrative access).
- Persistent buffer across restarts (append-only log, SQLite, or BadgerDB).
- Rate-limit handling (respect `X-RateLimit-*` headers from Mastodon REST).
- Media proxying / caching (avoid hotlinking Mastodon media URLs).
- CI/CD pipeline (GitHub Actions: lint → test → docker push).
- Test strategy: unit tests for buffer and ingest; integration tests with a mock
  Mastodon streaming server; end-to-end tests with Playwright.

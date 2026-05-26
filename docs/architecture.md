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
┌────────────────────────────────────────────────────────┐
│  internal/api  — HTTP handlers                         │
│  GET /api/timeline   GET /api/search   GET /api/stream │
│  GET /healthz        GET /readyz       GET /statusz    │
│  GET /*  → embedded SPA (internal/static)              │
└────────────────────────────────────────────────────────┘
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

A single goroutine that consumes a stream of `*model.Event` from the Mastodon client and coordinates storage, indexing, and live streaming:

1. **Ring Buffer Integration**: 
   - **New Statuses**: Synchronously added to the ring buffer (with O(1) duplicate checks).
   - **Edits**: Synchronously updated in place.
   - **Deletions**: Synchronously removed by status ID.
2. **Search Index Batching & Deletion**:
   - New and edited statuses are accumulated and batch-written (upserted) to MeiliSearch when either **1 second has elapsed** or **100 statuses are queued**, whichever comes first.
   - Upon receiving a deletion event, the pipeline flushes the current index batch first to preserve correct order, then immediately deletes the status by ID from the MeiliSearch index.
3. **SSE Fan-Out**:
   - Broadcasts only new statuses (`EventTypeStatus`) to all active, registered SSE subscriber channels. Slow clients that block are dropped using non-blocking channel sends to prevent pipeline backpressure.
4. **State Persistence**:
   - The pipeline tracks the highest snowflake ID processed so far. After writing a batch to MeiliSearch, it persists this `since_id` state in MeiliSearch (under the `minidon_state` index and `pagination` document ID) for crash recovery.

`Subscribe` / `Unsubscribe` methods allow the HTTP stream handler to register and deregister client channels safely under a read-write mutex.

### Ring Buffer (`internal/buffer`)

Bounded, in-memory, thread-safe cache of recent statuses.

- Default capacity: 500 items; configurable via `MINIDON_BUFFER_SIZE`.
- Eviction: oldest entry dropped when capacity is exceeded.
- Interface methods:
  - `Add(s *model.Status) bool`: Appends a status. Avoids duplicates using a lookup map.
  - `Update(s *model.Status) bool`: Updates an existing status in place.
  - `Delete(id string) bool`: Removes a status by ID.
  - `Recent(n int) []*model.Status`: Returns the `n` most recent statuses in reverse chronological order.
- Concurrency design: A write mutex (`sync.Mutex`) protects writes (`Add`, `Update`, `Delete`) and internal slices. Reads (`Recent`) are lock-free and query an immutable snapshot of statuses stored in an `atomic.Pointer[[]*model.Status]`, which is swapped after every write operation. An internal `map[string]struct{}` provides $O(1)$ duplicate checks and lookup/eviction capability.

### Index (`internal/index`)

Interface:

```go
type Index interface {
	Index(statuses []model.Status) error
	Delete(ctx context.Context, id string) error
	Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error)
	EnsureSettings(ctx context.Context) error
	GetSinceID(ctx context.Context) (string, error)
	SaveSinceID(ctx context.Context, sinceID string) error
}
```

**MeiliSearch implementation** (`meili.go`):

- Primary index name: `statuses`.
- State index name: `minidon_state` (document ID `pagination` stores the `since_id` string).
- Searchable attributes: `content`, `account.acct`, `account.display_name`, `tags.name`.
- Sortable: `created_at`.
- Filterable: `language`, `tags.name`.
- **API Key Resolution**: Resolves the "Default Admin API Key" automatically using the master key provided via `MINIDON_MEILI_KEY`.
- **Ensure Settings**: Applies the searchable, sortable, and filterable configuration idempotently on startup. If MeiliSearch is booting up, it retries with a backoff (up to 5 attempts, waiting 2 seconds between attempts).

### HTTP API (`internal/api`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/timeline?limit=N` | Most-recent N statuses from the ring buffer (default 50, max 200) |
| GET | `/api/search?q=&limit=&offset=` | Full-text search via MeiliSearch |
| GET | `/api/stream` | SSE stream |
| GET | `/healthz` | Liveness probe — returns JSON status (always 200 OK) |
| GET | `/readyz` | Readiness probe — 200 OK (checks Mastodon connection status) |
| GET | `/statusz` | Status probe — returns detailed dependency status and stats |

Routes are registered using Go 1.22+ `http.ServeMux` method+pattern matching
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
make build  # Go 1.26: go build → bin/minidon (embeds web/dist)
```

The resulting binary is self-contained: `go:embed` bundles `web/dist` into the
executable at compile time. Running `./bin/minidon` (or `./bin/minidon web`) starts an HTTP server on
`:8080` (configurable via `--listen` or `MINIDON_LISTEN`) that serves the SPA and API routes. Alternatively, running `./bin/minidon cli` runs the streaming client CLI.

### Docker (Multi-Stage)

```
Stage 1: node:20-alpine   — npm ci && npm run build
Stage 2: golang:1.26-alpine — go build (copies web/dist from stage 1)
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

## 9. Decisions & Technical Tradeoffs

### Streaming Transport Choice
- **Decision**: Used the `mattn/go-mastodon` library. It simplifies authentication, token handling, and streaming protocol parsing. We built a robust reconnection wrapper around it with exponential backoff + jitter and a REST-based catch-up backfill. Integration tests simulate Mastodon stream endpoints to verify correct reconnection behavior.

### Buffer Deduplication on Reconnect
- **Decision**: Implemented duplicate filtering (skipping on duplicate ID using a thread-safe map inside `Buffer`). This prevents duplicate statuses from accumulating in the in-memory cache and MeiliSearch index during REST backfills.

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
- Persistent buffer across restarts (using an embedded DB like SQLite or BadgerDB).
- Rate-limit handling (respect `X-RateLimit-*` headers from Mastodon REST).
- Media proxying / caching (avoid hotlinking Mastodon media URLs).
- CI/CD pipeline (GitHub Actions: lint → test → docker push).
- End-to-end browser testing (e.g., using Playwright).

# minidon — Architecture

> **Draft** — this document reflects the planned architecture after the initial
> scaffolding pass.  No business logic is implemented yet.

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
| GET | `/api/stream` | SSE stream; each event is a JSON `model.Status` with type `"status"` |
| GET | `/healthz` | Liveness probe — always 200 OK |
| GET | `/readyz` | Readiness probe — 200 once Mastodon client is connected |
| GET | `/*` | Embedded SPA with `index.html` fallback for client-side routing |

### Static Assets (`internal/static`)

`//go:embed web/dist` embeds the compiled Vite output into the binary at build time.
Requests that do not match `/api/*` are served from the embedded FS, with
`index.html` returned for any path that does not correspond to a file (SPA fallback).

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

All settings via environment variables (12-factor):

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIDON_LISTEN` | `:8080` | TCP listen address |
| `MINIDON_MASTODON_INSTANCE` | *(required)* | Mastodon instance hostname |
| `MINIDON_MEILI_URL` | `http://localhost:7700` | MeiliSearch URL |
| `MINIDON_MEILI_KEY` | `""` | MeiliSearch API key |
| `MINIDON_BUFFER_SIZE` | `500` | Ring buffer capacity |

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

### SSE vs WebSocket for browser stream
- SSE is simpler (unidirectional, auto-reconnects, works through HTTP/1.1 proxies).
- WebSocket adds bidirectionality (not needed). **Decision: SSE.**

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

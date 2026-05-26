# minidon — HTTP API Reference

---

## Base URL

When running locally: `http://localhost:8080`

---

## Endpoints

### `GET /api/timeline`

Returns the most-recent statuses from the in-memory ring buffer.

**Query parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | `50` | Number of statuses to return (max 200) |

**Response** — `200 OK`, `application/json`

```json
[
  {
    "id": "...",
    "uri": "...",
    "url": "...",
    "created_at": "2024-01-01T00:00:00Z",
    "content": "<p>Hello, Mastodon!</p>",
    "language": "en",
    "account": {
      "acct": "user@mastodon.social",
      "display_name": "User",
      "avatar": "https://..."
    },
    "media_attachments": [],
    "tags": [],
    "reblog": null
  }
]
```

---

### `GET /api/search`

Full-text search over indexed statuses via MeiliSearch.

**Query parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `q` | string | *(required)* | Search query |
| `limit` | integer | `20` | Results per page (max 100) |
| `offset` | integer | `0` | Pagination offset |

**Response** — `200 OK`, `application/json`

```json
{
  "hits": [ /* array of Status objects */ ],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

---

### `GET /api/stream`

Server-Sent Events (SSE) stream of new statuses as they arrive from Mastodon.

**Response** — `200 OK`, `text/event-stream`

Each event:
```
event: status
data: {"id":"...","content":"...","created_at":"..."}

```

Clients should use the browser `EventSource` API:
```js
const es = new EventSource('/api/stream')
es.addEventListener('status', (e) => {
  const status = JSON.parse(e.data)
})
```

---

### `GET /healthz`

Liveness probe. Returns JSON payload describing the service status and uptime. Always returns `200 OK` if the process is running.

**Response** — `200 OK`, `application/json`

```json
{
  "status": "healthy",
  "initialized": true,
  "uptime": "10s"
}
```

---

### `GET /readyz`

Readiness probe. Returns `200 OK` once the Mastodon streaming client is
connected; returns `503 Service Unavailable` before that.

---

### `GET /statusz`

Status probe. Returns detailed JSON status representing internal dependency health, server configurations, and index statistics.

**Response** — `200 OK`, `application/json`

```json
{
  "dependencies": {
    "mastodon": {
      "connected": true,
      "server": "https://mastodon.social",
      "stream": "public"
    },
    "meilisearch": {
      "enabled": true,
      "connected": true,
      "url": "http://localhost:7700",
      "stats": {
        "databaseSize": 1234,
        "lastUpdate": "2026-05-26T00:09:19Z",
        "indexes": {
          "statuses": {
            "numberOfDocuments": 42
          }
        }
      }
    }
  }
}
```

---

### `GET /*`

All other paths serve the embedded React SPA. Unknown paths return `index.html`
to support client-side routing.

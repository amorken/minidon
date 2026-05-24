# minidon — Implementation Progress

This document tracks the implementation progress of `minidon` components against the planned design.

## Implementation Gap Analysis (Planned vs. Built)

While the documentation outlines a complete architecture, the implementation is currently in an **early scaffolding state**. Below is a summary of what is built versus what is missing:

| Component | Status | Location | Notes |
| :--- | :--- | :--- | :--- |
| **Data Model** | **Complete** | [internal/model/status.go](../internal/model/status.go) | Defines the shared `Status` and related DTOs. |
| **Config Loader** | **Complete** | [internal/config/config.go](../internal/config/config.go) | Loads settings from environment variables, including disabling search. |
| **Static Assets** | **Complete** | [internal/static/static.go](../internal/static/static.go) | Implements embedding/serving with caching policies. |
| **Mastodon Client** | **Partial** | [internal/mastodon/client.go](../internal/mastodon/client.go) | Streaming connection is built. **Missing**: Periodic REST fallback (backfill) on reconnect to prevent gaps in timeline. Not yet wired in `main.go`. |
| **Ring Buffer** | **Stub** | [internal/buffer/buffer.go](../internal/buffer/buffer.go) | Contains only package-level doc comments. Needs struct and implementation. |
| **Search Index** | **Stub** | [internal/index/](../internal/index/) | `index.go` and `meili.go` contain only package-level comments. Needs interface, MeiliSearch client wrapper, settings initialization, and search methods. |
| **Ingest Pipeline** | **Stub** | [internal/ingest/ingest.go](../internal/ingest/ingest.go) | Contains only package-level comments. Needs fan-out goroutine, debounced indexing, and thread-safe SSE subscription management. |
| **HTTP API Router** | **Partial** | [internal/api/router.go](../internal/api/router.go) | Router is defined, but `/api/timeline`, `/api/search`, and `/api/stream` handlers return `501 Not Implemented`. |
| **HTTP API Handlers**| **Stub** | [internal/api/](../internal/api/) | `search.go`, `stream.go`, `timeline.go` contain only comments. Handlers need to be written. |
| **Main Entrypoint** | **Stub** | [cmd/minidon/main.go](../cmd/minidon/main.go) | Sets up router and starts HTTP server. **Missing**: Component initialization (Mastodon client, buffer, index, ingest pipeline), lifecycle wiring, and graceful shutdown integration. |
| **Frontend SPA** | **Stub** | [web/src/App.tsx](../web/src/App.tsx) | Skeleton Vite setup. `App.tsx` contains only a placeholder page. Needs timeline, search, details views, and SSE client wrapper. |

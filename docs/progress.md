# minidon — Implementation Progress

This document tracks the implementation progress of `minidon` components against the planned design.

## Implementation Status

The codebase is fully integrated with working implementations of the planned backend and frontend components. However, additional verification, testing, and handling of edge cases (such as rate limits and reconnection states) may require further refinement.

| Component | Status | Location | Notes |
| :--- | :--- | :--- | :--- |
| **Data Model** | **Complete** | [internal/model/status.go](../internal/model/status.go) | Defines the shared `Status` and related DTOs. |
| **Config Loader** | **Complete** | [internal/config/config.go](../internal/config/config.go) | Loads settings from CLI flags and environment variables via Kong. |
| **Static Assets** | **Complete** | [internal/static/static.go](../internal/static/static.go) | Implements embedding/serving with caching policies. |
| **Mastodon Client** | **Complete** | [internal/mastodon/client.go](../internal/mastodon/client.go) | Real-time WebSocket streaming with periodic REST backfill on reconnect. |
| **Ring Buffer** | **Complete** | [internal/buffer/buffer.go](../internal/buffer/buffer.go) | Thread-safe, bounded in-memory buffer with lock-free atomic snapshot reads. |
| **Search Index** | **Complete** | [internal/index/](../internal/index/) | Handles full-text search indexing with MeiliSearch and fallback NoopIndex. |
| **Ingest Pipeline** | **Complete** | [internal/ingest/ingest.go](../internal/ingest/ingest.go) | Orchestrates fan-out streaming, debounced search index batching, and client SSE registrations. |
| **HTTP API Router** | **Complete** | [internal/api/router.go](../internal/api/router.go) | Routes requests for timelines, searches, streams, and health probes. |
| **HTTP API Handlers**| **Complete** | [internal/api/](../internal/api/) | Full implementations for timeline, search, stream handlers, and health endpoints. |
| **Main Entrypoint** | **Complete** | [cmd/minidon/main.go](../cmd/minidon/main.go) | Wires component initialization, CLI/Web/Delete-Index modes, and graceful shutdown lifecycle. |
| **Frontend SPA** | **Complete** | [web/src/App.tsx](../web/src/App.tsx) | App UI with timeline streams, debounced search, detailed modals, and connection indicators. |

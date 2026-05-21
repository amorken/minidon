// Package ingest implements the fan-out pipeline that sits between the
// Mastodon streaming client and the downstream consumers.
//
// A single goroutine reads statuses from the mastodon.Client channel and
// writes them concurrently to:
//   - the in-memory ring buffer (internal/buffer)
//   - the MeiliSearch batch writer (internal/index), debounced (flush every
//     1 s or 100 documents, whichever comes first)
//   - all active SSE subscribers registered with the HTTP stream handler
//     (internal/api)
//
// TODO: define Pipeline struct, Start(ctx), and Subscribe/Unsubscribe methods
// for SSE fan-out.
package ingest

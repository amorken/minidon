// Package api contains the HTTP handlers and router for minidon's REST/SSE API.
//
// Routes:
//   GET /api/timeline?limit=N    — most-recent N statuses from the ring buffer
//   GET /api/search?q=&limit=&offset=  — full-text search via MeiliSearch
//   GET /api/stream              — Server-Sent Events stream of new statuses
//   GET /healthz                 — liveness probe (always 200 OK)
//   GET /readyz                  — readiness probe (200 once Mastodon connected)
//   GET /*                       — embedded SPA (index.html fallback)
//
// TODO: implement NewRouter(deps) *http.ServeMux wiring all handlers.
package api

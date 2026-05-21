// Package static serves the compiled React/Vite SPA embedded directly in
// the Go binary via go:embed.
//
// The embedded filesystem is rooted at web/dist (relative to the module
// root).  All requests that do not match an API route are served from this
// filesystem, with a fallback to index.html for client-side routing support.
//
// TODO: add //go:embed web/dist directive and implement Handler() http.Handler
// with SPA fallback.
package static

// Package api — SSE stream handler.
//
// GET /api/stream upgrades the connection to a Server-Sent Events stream and
// forwards new statuses posted by the ingest pipeline to the browser.
// Each event is a JSON-encoded model.Status with event type "status".
//
// Clients that disconnect are automatically unregistered from the fan-out
// to avoid goroutine leaks.
//
// TODO: implement streamHandler(pipeline *ingest.Pipeline) http.HandlerFunc.
package api

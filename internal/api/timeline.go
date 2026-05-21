// Package api — timeline handler.
//
// GET /api/timeline?limit=N returns the N most-recent statuses from the
// in-memory ring buffer as a JSON array.  Default limit: 50, max: 200.
//
// TODO: implement timelineHandler(buf *buffer.Buffer) http.HandlerFunc.
package api

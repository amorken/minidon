// Package api — search handler.
//
// GET /api/search?q=<query>&limit=<n>&offset=<n> proxies the query to the
// Index backend and returns matching statuses as a JSON object with
// "hits", "total", "limit", and "offset" fields.
//
// TODO: implement searchHandler(idx index.Index) http.HandlerFunc.
package api

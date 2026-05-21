// Package index defines the Index interface that abstracts the full-text
// search backend used by minidon.
//
// The primary implementation is MeiliSearch (see meili.go), but the
// interface allows alternative backends (Bleve, Typesense, SQLite FTS5)
// to be plugged in without changes to the ingest pipeline or HTTP handlers.
//
// TODO: define Index interface with Index(statuses []model.Status) error
// and Search(query string, opts SearchOptions) ([]model.Status, error).
package index

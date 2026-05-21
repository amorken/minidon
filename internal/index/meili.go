// Package index — MeiliSearch implementation of the Index interface.
//
// The MeiliSearch index is named "statuses".  Searchable attributes:
// content, account.acct, account.display_name, tags.name.
// Sortable: created_at.  Filterable: language, tags.name.
//
// Documents are written in batches (debounced by the ingest pipeline) to
// amortise HTTP round-trips to MeiliSearch.
//
// TODO: implement meiliIndex struct using the meilisearch-go SDK.
// TODO: add EnsureSettings() to apply searchable/sortable/filterable config
// idempotently on startup.
package index

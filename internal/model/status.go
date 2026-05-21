// Package model defines the data transfer objects (DTOs) shared between
// the ingest pipeline, the ring buffer, the search index, and the HTTP API.
//
// The Status type is a subset of the Mastodon Status entity that minidon
// persists and serves.  Fields included:
//
//   - ID, URI, URL
//   - CreatedAt
//   - Content (sanitised HTML or plain text)
//   - Language
//   - Account (Acct, DisplayName, Avatar)
//   - MediaAttachments (PreviewURL, Type)
//   - Tags (Name)
//   - Reblog (recursive, depth 1)
//
// TODO: define Status, Account, MediaAttachment, Tag structs with JSON tags.
package model

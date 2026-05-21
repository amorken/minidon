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
package model

import "time"

type Status struct {
	ID               string            `json:"id"`
	URI              string            `json:"uri"`
	URL              string            `json:"url"`
	CreatedAt        time.Time         `json:"created_at"`
	Content          string            `json:"content"`
	Language         string            `json:"language"`
	Account          Account           `json:"account"`
	MediaAttachments []MediaAttachment `json:"media_attachments"`
	Tags             []Tag             `json:"tags"`
	Reblog           *Status           `json:"reblog"`
}

type Account struct {
	Acct        string `json:"acct"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
}

type MediaAttachment struct {
	PreviewURL string `json:"preview_url"`
	Type       string `json:"type"`
}

type Tag struct {
	Name string `json:"name"`
}

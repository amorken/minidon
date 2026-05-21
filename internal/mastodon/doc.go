// Package mastodon provides a client for the Mastodon public streaming API.
//
// The primary interface, Client, abstracts the underlying transport so that
// the mattn/go-mastodon WebSocket wrapper or a hand-rolled REST/WS fallback
// can be swapped without touching the rest of the codebase.
//
// On connect the client subscribes to the public timeline stream
// (wss://<instance>/api/v1/streaming?stream=public) and exposes incoming
// statuses as a read-only channel.  Reconnection with exponential back-off
// is handled internally.
package mastodon

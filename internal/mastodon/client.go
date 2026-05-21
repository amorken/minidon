// Package mastodon — see doc.go for package-level documentation.
//
// This file declares the Client interface and the concrete implementation
// backed by mattn/go-mastodon.
//
// TODO: define Client interface with Connect(ctx) and Statuses() <-chan *model.Status methods.
// TODO: implement mastodonClient struct using go-mastodon streaming.
// TODO: add reconnect loop with exponential back-off.
package mastodon

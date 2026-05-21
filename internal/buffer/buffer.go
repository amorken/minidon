// Package buffer provides a thread-safe, bounded ring buffer of recent
// Mastodon statuses held in memory.
//
// The buffer maintains at most N statuses (default 500, configurable via
// MINIDON_BUFFER_SIZE).  Oldest entries are evicted when capacity is
// exceeded.  Recent(n) returns the n most-recent entries in reverse
// chronological order in O(n) time.
//
// Concurrent access is safe: writes use a sync.Mutex; reads use a snapshot
// approach (atomic.Pointer[[]Status]) to avoid blocking writers during
// typical read bursts.
//
// TODO: define Buffer struct, New(size int) constructor, Add(*model.Status),
// and Recent(n int) []model.Status.
package buffer

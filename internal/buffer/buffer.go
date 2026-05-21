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
package buffer

import (
	"sync"
	"sync/atomic"

	"github.com/amorken/minidon/internal/model"
)

type Buffer struct {
	mu       sync.Mutex
	size     int
	data     []model.Status
	head     int // Next write index
	count    int // Total items in buffer
	snapshot atomic.Pointer[[]model.Status]
}

// New creates a new Buffer with the given capacity.
func New(size int) *Buffer {
	if size <= 0 {
		size = 500
	}
	b := &Buffer{
		size: size,
		data: make([]model.Status, size),
	}
	empty := make([]model.Status, 0)
	b.snapshot.Store(&empty)
	return b
}

// Add appends a new status to the ring buffer and updates the atomic snapshot.
func (b *Buffer) Add(s *model.Status) {
	if s == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.data[b.head] = *s
	b.head = (b.head + 1) % b.size
	if b.count < b.size {
		b.count++
	}

	// Generate a new reverse-chronological snapshot
	snap := make([]model.Status, b.count)
	idx := b.head
	for i := 0; i < b.count; i++ {
		idx = (idx - 1 + b.size) % b.size
		snap[i] = b.data[idx]
	}

	b.snapshot.Store(&snap)
}

// Recent returns the n most-recent statuses in reverse chronological order.
func (b *Buffer) Recent(n int) []model.Status {
	if n <= 0 {
		return nil
	}

	snapPtr := b.snapshot.Load()
	if snapPtr == nil {
		return nil
	}

	snap := *snapPtr
	if n > len(snap) {
		n = len(snap)
	}

	res := make([]model.Status, n)
	copy(res, snap[:n])
	return res
}

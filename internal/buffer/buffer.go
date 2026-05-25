// Package buffer implements a thread-safe, bounded, in-memory ring buffer
// for caching recent Mastodon status updates.
package buffer

import (
	"sync"
	"sync/atomic"

	"github.com/amorken/minidon/internal/model"
)

type Buffer struct {
	mu       sync.Mutex
	capacity int
	statuses []*model.Status
	ids      map[string]struct{}
	snapshot atomic.Pointer[[]*model.Status]
}

// New constructs a new Buffer with the given capacity.
func New(size int) *Buffer {
	if size <= 0 {
		size = 500
	}
	b := &Buffer{
		capacity: size,
		statuses: make([]*model.Status, 0, size),
		ids:      make(map[string]struct{}),
	}
	empty := make([]*model.Status, 0)
	b.snapshot.Store(&empty)
	return b
}

// Add appends a status to the buffer. If the status is a duplicate, it is ignored and returns false.
// If capacity is reached, the oldest status is evicted.
func (b *Buffer) Add(s *model.Status) bool {
	if s == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// O(1) duplicate check
	if _, exists := b.ids[s.ID]; exists {
		return false
	}

	// Evict oldest if capacity is reached
	if len(b.statuses) >= b.capacity {
		oldest := b.statuses[0]
		delete(b.ids, oldest.ID)
		b.statuses = b.statuses[1:]
	}

	b.statuses = append(b.statuses, s)
	b.ids[s.ID] = struct{}{}

	// Create reverse-chronological snapshot for lock-free reads
	snap := make([]*model.Status, len(b.statuses))
	for i, st := range b.statuses {
		snap[len(b.statuses)-1-i] = st
	}
	b.snapshot.Store(&snap)

	return true
}

// Update replaces the status in the buffer if it already exists. Returns true if updated.
func (b *Buffer) Update(s *model.Status) bool {
	if s == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.ids[s.ID]; !exists {
		return false
	}

	for i, st := range b.statuses {
		if st.ID == s.ID {
			b.statuses[i] = s
			break
		}
	}

	// Create reverse-chronological snapshot for lock-free reads
	snap := make([]*model.Status, len(b.statuses))
	for i, st := range b.statuses {
		snap[len(b.statuses)-1-i] = st
	}
	b.snapshot.Store(&snap)

	return true
}

// Delete removes the status with the given ID from the buffer. Returns true if deleted.
func (b *Buffer) Delete(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.ids[id]; !exists {
		return false
	}

	delete(b.ids, id)

	for i, st := range b.statuses {
		if st.ID == id {
			b.statuses = append(b.statuses[:i], b.statuses[i+1:]...)
			break
		}
	}

	// Create reverse-chronological snapshot for lock-free reads
	snap := make([]*model.Status, len(b.statuses))
	for i, st := range b.statuses {
		snap[len(b.statuses)-1-i] = st
	}
	b.snapshot.Store(&snap)

	return true
}

// Recent returns the n most-recent statuses in reverse-chronological order.
func (b *Buffer) Recent(n int) []*model.Status {
	snapPtr := b.snapshot.Load()
	if snapPtr == nil {
		return nil
	}
	snap := *snapPtr
	if n <= 0 {
		return nil
	}
	if n > len(snap) {
		n = len(snap)
	}
	return snap[:n]
}


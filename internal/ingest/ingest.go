// Package ingest implements the fan-out ingestion pipeline consuming real-time events
// from the Mastodon client and broadcasting them to internal storage and subscribers.
package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/model"
)

type Pipeline struct {
	src         <-chan *model.Status
	buffer      *buffer.Buffer
	idx         index.Index
	mu          sync.RWMutex
	subscribers map[chan *model.Status]struct{}
}

// New constructs a new ingest Pipeline.
func New(src <-chan *model.Status, buf *buffer.Buffer, idx index.Index) *Pipeline {
	return &Pipeline{
		src:         src,
		buffer:      buf,
		idx:         idx,
		subscribers: make(map[chan *model.Status]struct{}),
	}
}

// Subscribe registers a new subscriber channel.
func (p *Pipeline) Subscribe() chan *model.Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan *model.Status, 128)
	p.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe deregisters a subscriber channel without closing it.
// Letting the channel garbage collect prevents send-on-closed-channel panics.
func (p *Pipeline) Unsubscribe(ch chan *model.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subscribers, ch)
}

// Start runs the ingest pipeline processing loop. It consumes statuses from the
// source client channel and distributes them to the ring buffer, search index, and subscribers.
func (p *Pipeline) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var batch []model.Status
	const maxBatchSize = 100

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := p.idx.Index(batch); err != nil {
			slog.Error("ingest pipeline failed to index batch", "err", err)
			batch = batch[:0]
			return
		}
		slog.Debug("ingest pipeline indexed batch", "count", len(batch))
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-ticker.C:
			flush()
		case status, ok := <-p.src:
			if !ok {
				flush()
				return
			}

			slog.Debug("ingest pipeline received status", "id", status.ID)

			// Immediate synchronous write to ring buffer with duplicate filtering
			added := p.buffer.Add(status)
			if !added {
				continue
			}

			// Add to index batch
			batch = append(batch, *status)
			if len(batch) >= maxBatchSize {
				flush()
			}

			// Broadcast to active SSE subscribers
			p.mu.RLock()
			for ch := range p.subscribers {
				select {
				case ch <- status:
				default:
					slog.Debug("dropping status for slow subscriber channel")
				}
			}
			p.mu.RUnlock()
		}
	}
}

// Package ingest implements the fan-out pipeline that sits between the
// Mastodon streaming client and the downstream consumers.
//
// A single goroutine reads statuses from the mastodon.Client channel and
// writes them concurrently to:
//   - the in-memory ring buffer (internal/buffer)
//   - the MeiliSearch batch writer (internal/index), debounced (flush every
//     1 s or 100 documents, whichever comes first)
//   - all active SSE subscribers registered with the HTTP stream handler
//     (internal/api)
package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/mastodon"
	"github.com/amorken/minidon/internal/model"
)

type Pipeline struct {
	client      mastodon.Client
	buffer      *buffer.Buffer
	idx         index.Index
	mu          sync.Mutex
	subscribers map[chan *model.Status]struct{}
	indexChan   chan model.Status
}

// NewPipeline creates and initializes a new Pipeline.
func NewPipeline(client mastodon.Client, buf *buffer.Buffer, idx index.Index) *Pipeline {
	return &Pipeline{
		client:      client,
		buffer:      buf,
		idx:         idx,
		subscribers: make(map[chan *model.Status]struct{}),
		indexChan:   make(chan model.Status, 1000),
	}
}

// Subscribe registers a new subscriber channel and returns it.
func (p *Pipeline) Subscribe() chan *model.Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan *model.Status, 256)
	p.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes and closes a subscriber channel.
func (p *Pipeline) Unsubscribe(ch chan *model.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.subscribers[ch]; exists {
		delete(p.subscribers, ch)
		close(ch)
	}
}

// Start starts the ingest pipeline and the background indexer.
func (p *Pipeline) Start(ctx context.Context) error {
	// Start the background batch indexer
	go p.runIndexer(ctx)

	// Connect Mastodon client
	if err := p.client.Connect(ctx); err != nil {
		return err
	}

	// Main fan-out loop
	go p.runFanOut(ctx)

	return nil
}

func (p *Pipeline) runFanOut(ctx context.Context) {
	slog.Info("starting ingest pipeline fan-out loop")
	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping ingest pipeline fan-out loop")
			return
		case status, ok := <-p.client.Statuses():
			if !ok {
				slog.Info("mastodon statuses channel closed; stopping ingest pipeline")
				return
			}

			if status == nil {
				continue
			}

			// 1. Write to the in-memory ring buffer
			p.buffer.Add(status)

			// 2. Queue for MeiliSearch batch writer
			select {
			case p.indexChan <- *status:
			default:
				slog.Warn("ingest pipeline index queue full; status dropped from index queue", "id", status.ID)
			}

			// 3. Broadcast to all active SSE subscribers
			p.broadcast(status)
		}
	}
}

// Connected returns true if the underlying Mastodon client is connected.
func (p *Pipeline) Connected() bool {
	return p.client.Connected()
}

func (p *Pipeline) broadcast(status *model.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for ch := range p.subscribers {
		select {
		case ch <- status:
		default:
			slog.Warn("subscriber channel full; status dropped from subscriber channel")
		}
	}
}

func (p *Pipeline) runIndexer(ctx context.Context) {
	slog.Info("starting indexer batching loop")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	batch := make([]model.Status, 0, 100)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := p.idx.Index(batch); err != nil {
			slog.Error("failed to index batch to search backend", "err", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping indexer batching loop and flushing pending documents")
			flush()
			return
		case s := <-p.indexChan:
			batch = append(batch, s)
			if len(batch) >= 100 {
				flush()
				ticker.Reset(time.Second)
			}
		case <-ticker.C:
			flush()
		}
	}
}

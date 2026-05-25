package mastodon

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/amorken/minidon/internal/model"
)

type FakeClient struct {
	mu        sync.Mutex
	statuses  chan *model.Status
	closed    bool
	connected atomic.Bool
}

func NewFakeClient() *FakeClient {
	return &FakeClient{
		statuses: make(chan *model.Status, 256),
	}
}

func (f *FakeClient) Connect(_ context.Context) error {
	f.connected.Store(true)
	return nil
}

func (f *FakeClient) Statuses() <-chan *model.Status {
	return f.statuses
}

func (f *FakeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.statuses)
	}
	return nil
}

func (f *FakeClient) Send(s *model.Status) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	select {
	case f.statuses <- s:
		return true
	default:
		return false
	}
}

func (f *FakeClient) IsConnected() bool {
	return f.connected.Load()
}

func (f *FakeClient) IsClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

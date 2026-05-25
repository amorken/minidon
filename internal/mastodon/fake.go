package mastodon

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/amorken/minidon/internal/model"
)

type FakeClient struct {
	mu        sync.Mutex
	events    chan *model.Event
	closed    bool
	connected atomic.Bool
}

func NewFakeClient() *FakeClient {
	return &FakeClient{
		events: make(chan *model.Event, 256),
	}
}

func (f *FakeClient) Connect(_ context.Context) error {
	f.connected.Store(true)
	return nil
}

func (f *FakeClient) Events() <-chan *model.Event {
	return f.events
}

func (f *FakeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.events)
	}
	return nil
}

func (f *FakeClient) Send(s *model.Status) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	ev := &model.Event{
		Type:   model.EventTypeStatus,
		Status: s,
	}
	select {
	case f.events <- ev:
		return true
	default:
		return false
	}
}

func (f *FakeClient) SendEvent(ev *model.Event) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	select {
	case f.events <- ev:
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

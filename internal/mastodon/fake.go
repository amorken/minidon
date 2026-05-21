package mastodon

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/amorken/minidon/internal/model"
)

type FakeClient struct {
	statuses  chan *model.Status
	closed    atomic.Bool
	once      sync.Once
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
	f.closed.Store(true)
	f.once.Do(func() { close(f.statuses) })
	return nil
}

func (f *FakeClient) Send(s *model.Status) bool {
	if f.closed.Load() {
		return false
	}
	select {
	case f.statuses <- s:
		return true
	default:
		return false
	}
}

func (f *FakeClient) Connected() bool {
	return f.IsConnected()
}

func (f *FakeClient) IsConnected() bool {
	return f.connected.Load() && !f.closed.Load()
}

func (f *FakeClient) IsClosed() bool {
	return f.closed.Load()
}

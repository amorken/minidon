package ingest_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/model"
)

type mockIndex struct {
	mu      sync.Mutex
	indexed []model.Status
}

func (m *mockIndex) Index(statuses []model.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.indexed = append(m.indexed, statuses...)
	return nil
}

func (m *mockIndex) Search(ctx context.Context, query string, opts index.SearchOptions) (index.SearchResult, error) {
	return index.SearchResult{}, nil
}

func (m *mockIndex) EnsureSettings(ctx context.Context) error {
	return nil
}

func TestPipeline_Start(t *testing.T) {
	src := make(chan *model.Status, 10)
	buf := buffer.New(5)
	idx := &mockIndex{}
	p := ingest.New(src, buf, idx)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.Start(ctx)

	subCh := p.Subscribe()

	status := &model.Status{ID: "1", Content: "Hello"}
	src <- status

	// Check subscriber received the status
	select {
	case s := <-subCh:
		if s.ID != "1" {
			t.Errorf("expected status ID 1, got %s", s.ID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for subscriber status")
	}

	// Check buffer has the status
	recent := buf.Recent(1)
	if len(recent) != 1 || recent[0].ID != "1" {
		t.Fatalf("expected status ID 1 in buffer")
	}

	// Unsubscribe
	p.Unsubscribe(subCh)

	// Sending another status
	src <- &model.Status{ID: "2", Content: "World"}

	// Subscriber should NOT receive it because they unsubscribed
	select {
	case s, ok := <-subCh:
		if ok && s.ID == "2" {
			t.Error("received status after unsubscription")
		}
	case <-time.After(100 * time.Millisecond):
		// Expected to time out without receiving
	}

	// Wait for index debounce ticker (flushes every 1s)
	time.Sleep(1200 * time.Millisecond)

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if len(idx.indexed) != 2 {
		t.Errorf("expected 2 items indexed, got %d", len(idx.indexed))
	}
}

func TestPipeline_BatchFlush(t *testing.T) {
	src := make(chan *model.Status, 150)
	buf := buffer.New(200)
	idx := &mockIndex{}
	p := ingest.New(src, buf, idx)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.Start(ctx)

	// Send 101 statuses (should trigger immediate flush at 100)
	for i := 0; i < 101; i++ {
		src <- &model.Status{ID: strconv.Itoa(i), Content: "status"}
	}

	// Wait a small bit for processing
	time.Sleep(100 * time.Millisecond)

	idx.mu.Lock()
	indexedCount := len(idx.indexed)
	idx.mu.Unlock()

	if indexedCount < 100 {
		t.Errorf("expected at least 100 items indexed immediately, got %d", indexedCount)
	}
}

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
	deleted []string
	sinceID string
}

func (m *mockIndex) Index(statuses []model.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.indexed = append(m.indexed, statuses...)
	return nil
}

func (m *mockIndex) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *mockIndex) Search(ctx context.Context, query string, opts index.SearchOptions) (index.SearchResult, error) {
	return index.SearchResult{}, nil
}

func (m *mockIndex) EnsureSettings(ctx context.Context) error {
	return nil
}

func (m *mockIndex) GetSinceID(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sinceID, nil
}

func (m *mockIndex) SaveSinceID(ctx context.Context, sinceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sinceID = sinceID
	return nil
}

func TestPipeline_Start(t *testing.T) {
	src := make(chan *model.Event, 10)
	buf := buffer.New(5)
	idx := &mockIndex{}
	p := ingest.New(src, buf, idx)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.Start(ctx)

	subCh := p.Subscribe()

	status := &model.Status{ID: "1", Content: "Hello"}
	src <- &model.Event{Type: model.EventTypeStatus, Status: status}

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
	src <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: "2", Content: "World"}}

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
	src := make(chan *model.Event, 150)
	buf := buffer.New(200)
	idx := &mockIndex{}
	p := ingest.New(src, buf, idx)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.Start(ctx)

	// Send 101 statuses (should trigger immediate flush at 100)
	for i := range 101 {
		src <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: strconv.Itoa(i), Content: "status"}}
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

func TestPipeline_EditAndDelete(t *testing.T) {
	src := make(chan *model.Event, 10)
	buf := buffer.New(5)
	idx := &mockIndex{}
	p := ingest.New(src, buf, idx)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.Start(ctx)

	// 1. Add status
	src <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: "test-id", Content: "Original content"}}

	time.Sleep(50 * time.Millisecond)
	recent := buf.Recent(5)
	if len(recent) != 1 || recent[0].Content != "Original content" {
		t.Errorf("expected original status in buffer, got %v", recent)
	}

	// 2. Edit status
	src <- &model.Event{Type: model.EventTypeStatusEdit, Status: &model.Status{ID: "test-id", Content: "Edited content"}}

	time.Sleep(50 * time.Millisecond)
	recent = buf.Recent(5)
	if len(recent) != 1 || recent[0].Content != "Edited content" {
		t.Errorf("expected edited status in buffer, got %v", recent)
	}

	// 3. Delete status
	src <- &model.Event{Type: model.EventTypeStatusDelete, StatusID: "test-id"}

	time.Sleep(50 * time.Millisecond)
	recent = buf.Recent(5)
	if len(recent) != 0 {
		t.Errorf("expected status to be deleted from buffer, got %v", recent)
	}

	idx.mu.Lock()
	deletedCount := len(idx.deleted)
	deletedId := ""
	if deletedCount > 0 {
		deletedId = idx.deleted[0]
	}
	idx.mu.Unlock()

	if deletedCount != 1 || deletedId != "test-id" {
		t.Errorf("expected test-id to be deleted from index, got deleted count %d, deleted ID %q", deletedCount, deletedId)
	}
}

func TestPipeline_SinceIDPersistence(t *testing.T) {
	// 1. Initial backfill without sinceId
	src := make(chan *model.Event, 10)
	buf := buffer.New(10)
	idx := &mockIndex{} // starts with empty sinceID

	p := ingest.New(src, buf, idx)
	ctx, cancel := context.WithCancel(context.Background())

	// Send status events
	src <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: "10", Content: "Status 10"}}
	src <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: "12", Content: "Status 12"}}
	src <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: "11", Content: "Status 11"}}

	// Start pipeline
	go p.Start(ctx)

	// Wait briefly for processing, then cancel to flush
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for pipeline to finish
	time.Sleep(100 * time.Millisecond)

	idx.mu.Lock()
	savedSinceID := idx.sinceID
	idx.mu.Unlock()

	if savedSinceID != "12" {
		t.Errorf("expected since_id to be '12', got %q", savedSinceID)
	}

	// 2. Subsequent backfill / updates
	src2 := make(chan *model.Event, 10)
	buf2 := buffer.New(10)
	idx2 := &mockIndex{sinceID: "12"} // starts with loaded sinceID

	p2 := ingest.New(src2, buf2, idx2)
	ctx2, cancel2 := context.WithCancel(context.Background())

	// Send a newer status
	src2 <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: "15", Content: "Status 15"}}

	// Start pipeline
	go p2.Start(ctx2)

	// Wait briefly for processing, then cancel to flush
	time.Sleep(100 * time.Millisecond)
	cancel2()

	// Wait for pipeline to finish
	time.Sleep(100 * time.Millisecond)

	idx2.mu.Lock()
	savedSinceID2 := idx2.sinceID
	idx2.mu.Unlock()

	if savedSinceID2 != "15" {
		t.Errorf("expected since_id to update to '15', got %q", savedSinceID2)
	}
}

package buffer_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/model"
)

func TestBuffer_AddAndRecent(t *testing.T) {
	b := buffer.New(3)

	s1 := &model.Status{ID: "1", Content: "First"}
	s2 := &model.Status{ID: "2", Content: "Second"}
	s3 := &model.Status{ID: "3", Content: "Third"}
	s4 := &model.Status{ID: "4", Content: "Fourth"}

	if !b.Add(s1) {
		t.Error("expected s1 to be added")
	}
	if !b.Add(s2) {
		t.Error("expected s2 to be added")
	}

	// Recent should return in reverse chronological order
	recent := b.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 items, got %d", len(recent))
	}
	if recent[0].ID != "2" || recent[1].ID != "1" {
		t.Errorf("incorrect order: got %s, then %s", recent[0].ID, recent[1].ID)
	}

	// Add s3 - capacity reached
	if !b.Add(s3) {
		t.Error("expected s3 to be added")
	}

	// Add s4 - oldest (s1) should be evicted
	if !b.Add(s4) {
		t.Error("expected s4 to be added")
	}

	recent = b.Recent(10) // requesting more than capacity/size
	if len(recent) != 3 {
		t.Fatalf("expected 3 items (max capacity), got %d", len(recent))
	}
	if recent[0].ID != "4" || recent[1].ID != "3" || recent[2].ID != "2" {
		t.Errorf("incorrect eviction or order after capacity limit")
	}

	// Verify s1 is gone
	for _, r := range recent {
		if r.ID == "1" {
			t.Error("s1 was not evicted")
		}
	}
}

func TestBuffer_DuplicateFiltering(t *testing.T) {
	b := buffer.New(5)
	s1 := &model.Status{ID: "1", Content: "First"}
	s1Dup := &model.Status{ID: "1", Content: "First Duplicate"}

	if !b.Add(s1) {
		t.Error("expected s1 to be added")
	}
	if b.Add(s1Dup) {
		t.Error("expected s1Dup to be ignored")
	}

	recent := b.Recent(5)
	if len(recent) != 1 {
		t.Fatalf("expected 1 item, got %d", len(recent))
	}
	if recent[0].Content != "First" {
		t.Errorf("expected original content 'First', got %q", recent[0].Content)
	}
}

func TestBuffer_ConcurrentAccess(t *testing.T) {
	b := buffer.New(100)
	var wg sync.WaitGroup

	// Concurrently add statuses
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b.Add(&model.Status{ID: strconv.Itoa(id), Content: "status"})
		}(i)
	}

	// Concurrently read statuses
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Recent(10)
		}()
	}

	wg.Wait()

	recent := b.Recent(100)
	if len(recent) != 50 {
		t.Errorf("expected 50 statuses in buffer, got %d", len(recent))
	}
}

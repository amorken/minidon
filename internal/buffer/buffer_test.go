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
	for i := range 50 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b.Add(&model.Status{ID: strconv.Itoa(id), Content: "status"})
		}(i)
	}

	// Concurrently read statuses
	for range 50 {
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

func TestBuffer_Update(t *testing.T) {
	b := buffer.New(5)
	s1 := &model.Status{ID: "1", Content: "Original"}
	s2 := &model.Status{ID: "2", Content: "Two"}
	b.Add(s1)
	b.Add(s2)

	sUpdate := &model.Status{ID: "1", Content: "Updated"}
	if !b.Update(sUpdate) {
		t.Error("expected Update to return true")
	}

	recent := b.Recent(5)
	if len(recent) != 2 {
		t.Fatalf("expected 2 items in buffer, got %d", len(recent))
	}
	// Order should be s2 (most recent) then sUpdate
	if recent[0].ID != "2" || recent[0].Content != "Two" {
		t.Errorf("expected s2 to be untouched at index 0, got %+v", recent[0])
	}
	if recent[1].ID != "1" || recent[1].Content != "Updated" {
		t.Errorf("expected s1 to be updated at index 1, got %+v", recent[1])
	}

	sNonexistent := &model.Status{ID: "99", Content: "Nonexistent"}
	if b.Update(sNonexistent) {
		t.Error("expected Update to return false for nonexistent status ID")
	}
}

func TestBuffer_Delete(t *testing.T) {
	b := buffer.New(5)
	s1 := &model.Status{ID: "1", Content: "One"}
	s2 := &model.Status{ID: "2", Content: "Two"}
	s3 := &model.Status{ID: "3", Content: "Three"}
	b.Add(s1)
	b.Add(s2)
	b.Add(s3)

	if !b.Delete("2") {
		t.Error("expected Delete to return true for existing status")
	}

	recent := b.Recent(5)
	if len(recent) != 2 {
		t.Fatalf("expected 2 items, got %d", len(recent))
	}
	// Order should be s3 (most recent) then s1
	if recent[0].ID != "3" || recent[0].Content != "Three" {
		t.Errorf("expected s3 to be untouched at index 0, got %+v", recent[0])
	}
	if recent[1].ID != "1" || recent[1].Content != "One" {
		t.Errorf("expected s1 to be untouched at index 1, got %+v", recent[1])
	}

	if b.Delete("99") {
		t.Error("expected Delete to return false for nonexistent status")
	}
}


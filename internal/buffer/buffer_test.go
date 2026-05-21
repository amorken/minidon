package buffer_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/model"
)

func TestBuffer_New(t *testing.T) {
	b := buffer.New(10)
	if b == nil {
		t.Fatal("expected buffer, got nil")
	}

	recent := b.Recent(5)
	if len(recent) != 0 {
		t.Errorf("expected 0 recent items, got %d", len(recent))
	}
}

func TestBuffer_AddAndRecent(t *testing.T) {
	b := buffer.New(3)

	b.Add(&model.Status{ID: "1"})
	b.Add(&model.Status{ID: "2"})

	recent := b.Recent(5)
	if len(recent) != 2 {
		t.Fatalf("expected 2 items, got %d", len(recent))
	}
	if recent[0].ID != "2" || recent[1].ID != "1" {
		t.Errorf("expected reverse chronological order, got ID order: %s, %s", recent[0].ID, recent[1].ID)
	}

	b.Add(&model.Status{ID: "3"})
	b.Add(&model.Status{ID: "4"}) // evicts "1"

	recent = b.Recent(5)
	if len(recent) != 3 {
		t.Fatalf("expected 3 items, got %d", len(recent))
	}
	if recent[0].ID != "4" || recent[1].ID != "3" || recent[2].ID != "2" {
		t.Errorf("expected IDs [4, 3, 2], got [%s, %s, %s]", recent[0].ID, recent[1].ID, recent[2].ID)
	}

	// Request less than total
	recent = b.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 items, got %d", len(recent))
	}
	if recent[0].ID != "4" || recent[1].ID != "3" {
		t.Errorf("expected IDs [4, 3], got [%s, %s]", recent[0].ID, recent[1].ID)
	}
}

func TestBuffer_Concurrency(t *testing.T) {
	b := buffer.New(100)
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				b.Add(&model.Status{ID: strconv.Itoa(writerID*100 + j)})
			}
		}(i)
	}

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = b.Recent(50)
			}
		}()
	}

	wg.Wait()

	recent := b.Recent(150)
	if len(recent) != 100 {
		t.Errorf("expected buffer to be full at 100 items, got %d", len(recent))
	}
}

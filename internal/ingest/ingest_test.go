package ingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/mastodon"
	"github.com/amorken/minidon/internal/model"
)

func TestPipeline_SubscribeUnsubscribe(t *testing.T) {
	fc := mastodon.NewFakeClient()
	buf := buffer.New(10)
	idx := &index.NoopIndex{}

	pipeline := ingest.NewPipeline(fc, buf, idx)

	ch := pipeline.Subscribe()
	if ch == nil {
		t.Fatal("expected subscriber channel, got nil")
	}

	pipeline.Unsubscribe(ch)

	// Verify channel is closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for channel close")
	}
}

func TestPipeline_FanOut(t *testing.T) {
	fc := mastodon.NewFakeClient()
	buf := buffer.New(10)
	idx := &index.NoopIndex{}

	pipeline := ingest.NewPipeline(fc, buf, idx)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("unexpected Start error: %v", err)
	}

	ch := pipeline.Subscribe()
	defer pipeline.Unsubscribe(ch)

	status := &model.Status{
		ID:      "test-1",
		Content: "test content",
	}

	if !fc.Send(status) {
		t.Fatal("failed to send status to fake client")
	}

	// Wait for the subscriber to receive the broadcast status
	select {
	case received := <-ch:
		if received.ID != "test-1" {
			t.Errorf("expected status ID %q, got %q", "test-1", received.ID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast")
	}

	// Verify buffer has the status
	recent := buf.Recent(1)
	if len(recent) != 1 || recent[0].ID != "test-1" {
		t.Errorf("expected buffer to contain 'test-1', got recent count %d", len(recent))
	}
}

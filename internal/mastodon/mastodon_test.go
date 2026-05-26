package mastodon

import (
	"context"
	"testing"
	"time"

	mstdn "github.com/mattn/go-mastodon"

	"github.com/amorken/minidon/internal/model"
)

func TestNewValidationMissingServer(t *testing.T) {
	_, err := New(Config{AccessToken: "token"})
	if err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestNewValidationMissingToken(t *testing.T) {
	_, err := New(Config{Server: "https://mastodon.social"})
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
}

func TestNewValid(t *testing.T) {
	_, err := New(Config{Server: "https://mastodon.social", AccessToken: "token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewValidationTrimsSpaces(t *testing.T) {
	c, err := New(Config{Server: "  https://mastodon.social  ", AccessToken: "  token  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mc := c.(*mastodonClient)
	if mc.cfg.Server != "https://mastodon.social" {
		t.Errorf("expected server to be trimmed, got %q", mc.cfg.Server)
	}
	if mc.cfg.AccessToken != "token" {
		t.Errorf("expected access token to be trimmed, got %q", mc.cfg.AccessToken)
	}
}

func TestConvertStatusNil(t *testing.T) {
	if convertStatus(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestConvertStatus(t *testing.T) {
	now := time.Now()
	s := &mstdn.Status{
		ID:        "123",
		URI:       "uri:123",
		URL:       "https://example.com/123",
		Content:   "<p>Hello</p>",
		Language:  "en",
		CreatedAt: now,
		Account: mstdn.Account{
			Acct:        "user@example.com",
			DisplayName: "User",
			Avatar:      "https://example.com/avatar.png",
		},
		MediaAttachments: []mstdn.Attachment{
			{PreviewURL: "https://example.com/preview.png", Type: "image"},
		},
		Tags: []mstdn.Tag{
			{Name: "golang"},
		},
		Reblog: &mstdn.Status{
			ID:      "456",
			Content: "<p>Boosted</p>",
			Account: mstdn.Account{
				Acct:        "other@example.com",
				DisplayName: "Other",
			},
		},
	}

	result := convertStatus(s)

	if result.ID != "123" {
		t.Errorf("ID = %q, want %q", result.ID, "123")
	}
	if result.URI != "uri:123" {
		t.Errorf("URI = %q, want %q", result.URI, "uri:123")
	}
	if result.Content != "<p>Hello</p>" {
		t.Errorf("Content = %q, want %q", result.Content, "<p>Hello</p>")
	}
	if result.Language != "en" {
		t.Errorf("Language = %q, want %q", result.Language, "en")
	}
	if !result.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", result.CreatedAt, now)
	}
	if result.Account.Acct != "user@example.com" {
		t.Errorf("Account.Acct = %q, want %q", result.Account.Acct, "user@example.com")
	}
	if result.Account.DisplayName != "User" {
		t.Errorf("Account.DisplayName = %q, want %q", result.Account.DisplayName, "User")
	}
	if result.Account.Avatar != "https://example.com/avatar.png" {
		t.Errorf("Account.Avatar = %q, want %q", result.Account.Avatar, "https://example.com/avatar.png")
	}
	if len(result.MediaAttachments) != 1 {
		t.Fatalf("MediaAttachments len = %d, want 1", len(result.MediaAttachments))
	}
	if result.MediaAttachments[0].PreviewURL != "https://example.com/preview.png" {
		t.Errorf("MediaAttachments[0].PreviewURL = %q, want %q", result.MediaAttachments[0].PreviewURL, "https://example.com/preview.png")
	}
	if result.MediaAttachments[0].Type != "image" {
		t.Errorf("MediaAttachments[0].Type = %q, want %q", result.MediaAttachments[0].Type, "image")
	}
	if len(result.Tags) != 1 {
		t.Fatalf("Tags len = %d, want 1", len(result.Tags))
	}
	if result.Tags[0].Name != "golang" {
		t.Errorf("Tags[0].Name = %q, want %q", result.Tags[0].Name, "golang")
	}
	if result.Reblog == nil {
		t.Fatal("Reblog is nil, want non-nil")
	}
	if result.Reblog.ID != "456" {
		t.Errorf("Reblog.ID = %q, want %q", result.Reblog.ID, "456")
	}
}

func TestConvertStatusEmptySlices(t *testing.T) {
	s := &mstdn.Status{
		ID:        "1",
		Account:   mstdn.Account{},
		CreatedAt: time.Now(),
	}

	result := convertStatus(s)

	if result.MediaAttachments == nil {
		t.Error("MediaAttachments is nil, want empty slice")
	}
	if len(result.MediaAttachments) != 0 {
		t.Errorf("MediaAttachments len = %d, want 0", len(result.MediaAttachments))
	}
	if result.Tags == nil {
		t.Error("Tags is nil, want empty slice")
	}
	if len(result.Tags) != 0 {
		t.Errorf("Tags len = %d, want 0", len(result.Tags))
	}
}

func TestFakeClient(t *testing.T) {
	fc := NewFakeClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := fc.Connect(ctx); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	if !fc.IsConnected() {
		t.Error("expected connected")
	}

	status := &model.Status{
		ID:      "test-1",
		Content: "hello world",
	}

	go func() {
		if !fc.Send(status) {
			t.Error("Send returned false")
		}
	}()

	select {
	case ev := <-fc.Events():
		if ev.Type != model.EventTypeStatus {
			t.Errorf("got EventType %q, want %q", ev.Type, model.EventTypeStatus)
		}
		if ev.Status.ID != "test-1" {
			t.Errorf("got ID %q, want %q", ev.Status.ID, "test-1")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for status event")
	}

	if err := fc.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !fc.IsClosed() {
		t.Error("expected closed")
	}
}

func TestFakeClientImplementsInterface(t *testing.T) {
	var _ Client = NewFakeClient()
}

func TestNextBackoff(t *testing.T) {
	b := 1 * time.Second
	for range 20 {
		b = nextBackoff(b)
	}
	if b != 60*time.Second {
		t.Errorf("expected backoff capped at 60s, got %v", b)
	}
}

func TestIsNewerID(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"", "", false},
		{"1", "", true},
		{"", "1", false},
		{"2", "1", true},
		{"1", "2", false},
		{"10", "2", true},
		{"2", "10", false},
		{"101", "100", true},
		{"100", "101", false},
	}

	for _, tt := range tests {
		got := model.IsNewerID(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("model.IsNewerID(%q, %q) = %v; want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestClient_Backpressure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mastodonClient directly with a channel of size 1
	out := make(chan *model.Event, 1)
	m := &mastodonClient{
		out:  out,
		done: make(chan struct{}),
	}

	events := make(chan mstdn.Event, 10)

	// Send two events to the events channel
	events <- &mstdn.UpdateEvent{
		Status: &mstdn.Status{
			ID: "status-1",
		},
	}
	events <- &mstdn.UpdateEvent{
		Status: &mstdn.Status{
			ID: "status-2",
		},
	}

	// We'll run drain in a goroutine.
	// Since out has capacity 1, the first status will fill the channel.
	// The second status should block.
	drainDone := make(chan bool)
	go func() {
		drainDone <- m.drain(ctx, events)
	}()

	// Wait a bit to ensure it had time to process and block
	time.Sleep(50 * time.Millisecond)

	// Check if drain has finished. It should still be running because it is blocked.
	select {
	case <-drainDone:
		t.Fatal("drain exited prematurely, expected it to block due to backpressure")
	default:
		// Expected to block
	}

	// Read one event from out. This should unblock the second send.
	select {
	case ev := <-out:
		if ev.Status.ID != "status-1" {
			t.Errorf("expected status-1, got %s", ev.Status.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting to read status-1 from out")
	}

	// Now read the second event.
	select {
	case ev := <-out:
		if ev.Status.ID != "status-2" {
			t.Errorf("expected status-2, got %s", ev.Status.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting to read status-2 from out")
	}

	// Now close events to let drain exit cleanly
	close(events)

	select {
	case result := <-drainDone:
		if !result {
			t.Error("expected drain to return true on clean close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for drain to exit after closing events channel")
	}
}

func TestClient_BackpressureCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *model.Event, 1)
	m := &mastodonClient{
		out:  out,
		done: make(chan struct{}),
	}

	events := make(chan mstdn.Event, 10)

	// Fill the out channel
	out <- &model.Event{Type: model.EventTypeStatus, Status: &model.Status{ID: "blocking-event"}}

	// Write to events to make drain try to send to the full out channel
	events <- &mstdn.UpdateEvent{
		Status: &mstdn.Status{
			ID: "status-after-full",
		},
	}

	drainDone := make(chan bool)
	go func() {
		drainDone <- m.drain(ctx, events)
	}()

	// Wait a bit to ensure it is blocked
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger abort
	cancel()

	select {
	case result := <-drainDone:
		if result {
			t.Error("expected drain to return false on context cancellation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for drain to exit after context cancellation")
	}
}

package mastodon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	mstdn "github.com/mattn/go-mastodon"
)

func TestMastodonClientIntegration(t *testing.T) {
	var mu sync.Mutex
	websocketConns := make(map[*websocket.Conn]struct{})
	var backfillRequestCount int

	// Upgrader for websockets
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// Create test status for REST API timeline backfill
	backfillStatus := &mstdn.Status{
		ID:        "backfill-1",
		URI:       "uri:backfill-1",
		Content:   "backfill status",
		CreatedAt: time.Now(),
		Account: mstdn.Account{
			Acct:        "backfill-user",
			DisplayName: "Backfiller",
		},
	}

	// Create test status for live stream
	liveStatus := &mstdn.Status{
		ID:        "live-1",
		URI:       "uri:live-1",
		Content:   "live stream status",
		CreatedAt: time.Now(),
		Account: mstdn.Account{
			Acct:        "live-user",
			DisplayName: "Liver",
		},
	}

	// Start test HTTP/WS server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/timelines/public" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				t.Errorf("expected Authorization header to be 'Bearer test-token', got %q", auth)
			}

			mu.Lock()
			backfillRequestCount++
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]*mstdn.Status{backfillStatus})
			return
		}

		if r.URL.Path == "/api/v1/streaming" {
			stream := r.URL.Query().Get("stream")
			token := r.URL.Query().Get("access_token")
			if stream != "public" {
				t.Errorf("expected stream query parameter to be 'public', got %q", stream)
			}
			if token != "test-token" {
				t.Errorf("expected access_token query parameter to be 'test-token', got %q", token)
			}

			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Logf("websocket upgrade error: %v", err)
				return
			}
			mu.Lock()
			websocketConns[conn] = struct{}{}
			mu.Unlock()
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Initialize the mastodonClient with the test server URL
	cfg := Config{
		Server:      ts.URL,
		AccessToken: "test-token",
		Stream:      "public",
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to the mock server
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer client.Close()

	// Verify the client is receiving backfill statuses
	statusesChan := client.Statuses()
	select {
	case status := <-statusesChan:
		if status.ID != "backfill-1" {
			t.Errorf("expected backfill status ID to be 'backfill-1', got %q", status.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for backfill status")
	}

	// Wait for client to connect to WebSocket and update status flag
	start := time.Now()
	var connected bool
	for time.Since(start) < 3*time.Second {
		if client.IsConnected() {
			connected = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !connected {
		t.Error("expected client to transition to connected state")
	}

	// Construct and send a live update event over WebSocket
	liveStatusJSON, err := json.Marshal(liveStatus)
	if err != nil {
		t.Fatalf("failed to marshal live status: %v", err)
	}

	type StreamEvent struct {
		Event   string `json:"event"`
		Payload string `json:"payload"`
	}
	event := StreamEvent{
		Event:   "update",
		Payload: string(liveStatusJSON),
	}

	mu.Lock()
	connsCount := len(websocketConns)
	mu.Unlock()
	if connsCount == 0 {
		t.Fatal("no active websocket connections on server side")
	}

	mu.Lock()
	for conn := range websocketConns {
		if err := conn.WriteJSON(event); err != nil {
			t.Logf("failed to write ws message: %v", err)
		}
	}
	mu.Unlock()

	// Verify WebSocket status is received
	select {
	case status := <-statusesChan:
		if status.ID != "live-1" {
			t.Errorf("expected live status ID to be 'live-1', got %q", status.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for live status")
	}

	// Test reconnection behavior
	// Close current connections to trigger client disconnect detection
	mu.Lock()
	for conn := range websocketConns {
		conn.Close()
		delete(websocketConns, conn)
	}
	mu.Unlock()

	// Wait for the client to reconnect
	start = time.Now()
	var reconnected bool
	for time.Since(start) < 5*time.Second {
		mu.Lock()
		connsCount = len(websocketConns)
		mu.Unlock()
		if connsCount > 0 {
			reconnected = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !reconnected {
		t.Fatal("timeout waiting for client to reconnect")
	}

	// Send another event over the reconnected WebSocket
	reconnectStatus := &mstdn.Status{
		ID:        "reconnect-1",
		URI:       "uri:reconnect-1",
		Content:   "reconnect status",
		CreatedAt: time.Now(),
		Account: mstdn.Account{
			Acct:        "reconnect-user",
			DisplayName: "Reconnector",
		},
	}
	reconnectStatusJSON, err := json.Marshal(reconnectStatus)
	if err != nil {
		t.Fatalf("failed to marshal reconnect status: %v", err)
	}

	reconnectEvent := StreamEvent{
		Event:   "update",
		Payload: string(reconnectStatusJSON),
	}

	mu.Lock()
	for conn := range websocketConns {
		if err := conn.WriteJSON(reconnectEvent); err != nil {
			t.Logf("failed to write ws message after reconnect: %v", err)
		}
	}
	mu.Unlock()

	// Verify client receives reconnect status from backfill or stream
	// Note: reconnection starts a new backfill, so we might receive another "backfill-1" status first.
	// We want to drain statuses until we find "reconnect-1" or timeout.
	foundReconnectStatus := false
	timeout := time.After(3 * time.Second)
	for !foundReconnectStatus {
		select {
		case status := <-statusesChan:
			if status.ID == "reconnect-1" {
				foundReconnectStatus = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for status after reconnect")
		}
	}

	// Verify that the mock server received at least two backfill requests (initial + reconnect)
	start = time.Now()
	var gotSecondBackfill bool
	for time.Since(start) < 3*time.Second {
		mu.Lock()
		reqs := backfillRequestCount
		mu.Unlock()
		if reqs >= 2 {
			gotSecondBackfill = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !gotSecondBackfill {
		mu.Lock()
		reqs := backfillRequestCount
		mu.Unlock()
		t.Errorf("expected at least 2 backfill requests, got %d", reqs)
	}
}

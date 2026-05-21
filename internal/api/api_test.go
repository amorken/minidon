package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amorken/minidon"
	"github.com/amorken/minidon/internal/api"
	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/mastodon"
	"github.com/amorken/minidon/internal/model"
)

func TestRouter_Healthz(t *testing.T) {
	fc := mastodon.NewFakeClient()
	buf := buffer.New(10)
	idx := &index.NoopIndex{}
	pipeline := ingest.NewPipeline(fc, buf, idx)

	router := api.NewRouter(minidon.StaticFS, pipeline, buf, idx)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestRouter_Readyz(t *testing.T) {
	fc := mastodon.NewFakeClient()
	buf := buffer.New(10)
	idx := &index.NoopIndex{}
	pipeline := ingest.NewPipeline(fc, buf, idx)

	router := api.NewRouter(minidon.StaticFS, pipeline, buf, idx)

	// Not connected initially
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}

	// Connected after starting pipeline (fake client connects immediately)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req)

	if rr2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr2.Code)
	}
}

func TestRouter_Timeline(t *testing.T) {
	fc := mastodon.NewFakeClient()
	buf := buffer.New(10)
	idx := &index.NoopIndex{}
	pipeline := ingest.NewPipeline(fc, buf, idx)

	buf.Add(&model.Status{ID: "status-1"})
	buf.Add(&model.Status{ID: "status-2"})

	router := api.NewRouter(minidon.StaticFS, pipeline, buf, idx)

	req := httptest.NewRequest(http.MethodGet, "/api/timeline?limit=1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var results []model.Status
	if err := json.Unmarshal(rr.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal timeline response: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 status due to limit, got %d", len(results))
	}

	if results[0].ID != "status-2" {
		t.Errorf("expected ID status-2, got %s", results[0].ID)
	}
}

func TestRouter_Search(t *testing.T) {
	fc := mastodon.NewFakeClient()
	buf := buffer.New(10)
	idx := &index.NoopIndex{} // returns empty SearchResult
	pipeline := ingest.NewPipeline(fc, buf, idx)

	router := api.NewRouter(minidon.StaticFS, pipeline, buf, idx)

	// Missing 'q' param
	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing query, got %d", rr.Code)
	}

	// Valid search
	req2 := httptest.NewRequest(http.MethodGet, "/api/search?q=test&limit=5&offset=2", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr2.Code)
	}

	var searchResult index.SearchResult
	if err := json.Unmarshal(rr2.Body.Bytes(), &searchResult); err != nil {
		t.Fatalf("failed to unmarshal search response: %v", err)
	}

	if searchResult.Limit != 5 {
		t.Errorf("expected limit 5, got %d", searchResult.Limit)
	}
	if searchResult.Offset != 2 {
		t.Errorf("expected offset 2, got %d", searchResult.Offset)
	}
}

func TestRouter_StreamSSE(t *testing.T) {
	fc := mastodon.NewFakeClient()
	buf := buffer.New(10)
	idx := &index.NoopIndex{}
	pipeline := ingest.NewPipeline(fc, buf, idx)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("unexpected pipeline start error: %v", err)
	}

	router := api.NewRouter(minidon.StaticFS, pipeline, buf, idx)

	req := httptest.NewRequest(http.MethodGet, "/api/stream", nil)
	ctxStream, cancelStream := context.WithCancel(context.Background())
	req = req.WithContext(ctxStream)

	rr := httptest.NewRecorder()

	go func() {
		router.ServeHTTP(rr, req)
	}()

	// Wait briefly for connection to start
	time.Sleep(50 * time.Millisecond)

	// Send a status via fake client
	status := &model.Status{
		ID:      "stream-status-123",
		Content: "stream content",
	}
	fc.Send(status)

	// Wait for event delivery
	time.Sleep(50 * time.Millisecond)

	cancelStream() // disconnect stream client

	// Wait for handler cleanup
	time.Sleep(50 * time.Millisecond)

	headers := rr.Header()
	if headers.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", headers.Get("Content-Type"))
	}

	body := rr.Body.String()
	if !strings.Contains(body, ": ok") {
		t.Error("expected body to contain SSE initial handshake")
	}
	if !strings.Contains(body, "event: status") {
		t.Error("expected body to contain SSE status event")
	}
	if !strings.Contains(body, "stream-status-123") {
		t.Error("expected body to contain status ID")
	}
}

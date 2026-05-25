package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amorken/minidon/internal/api"
	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/mastodon"
	"github.com/amorken/minidon/internal/model"
)

type mockSearchIndex struct {
	searchedQuery string
	searchOpts    index.SearchOptions
}

func (m *mockSearchIndex) Index(statuses []model.Status) error {
	return nil
}

func (m *mockSearchIndex) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockSearchIndex) Search(ctx context.Context, query string, opts index.SearchOptions) (index.SearchResult, error) {
	m.searchedQuery = query
	m.searchOpts = opts
	return index.SearchResult{
		Hits:   []model.Status{{ID: "match-1", Content: "Match"}},
		Total:  1,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}

func (m *mockSearchIndex) EnsureSettings(ctx context.Context) error {
	return nil
}

func TestReadyz(t *testing.T) {
	buf := buffer.New(10)
	idx := &mockSearchIndex{}
	fc := mastodon.NewFakeClient()
	pipe := ingest.New(fc.Events(), buf, idx)

	mux := api.NewRouter(api.RouterConfig{
		StaticFS: nil,
		Buffer:   buf,
		Index:    idx,
		Pipeline: pipe,
		Client:   fc,
	})

	// 1. Test readyz when disconnected
	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}

	// 2. Test readyz when connected
	_ = fc.Connect(context.Background())
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTimelineRoute(t *testing.T) {
	buf := buffer.New(10)
	idx := &mockSearchIndex{}
	fc := mastodon.NewFakeClient()
	pipe := ingest.New(fc.Events(), buf, idx)

	s := &model.Status{ID: "timeline-1", Content: "Hello"}
	buf.Add(s)

	mux := api.NewRouter(api.RouterConfig{
		StaticFS: nil,
		Buffer:   buf,
		Index:    idx,
		Pipeline: pipe,
		Client:   fc,
	})

	req := httptest.NewRequest("GET", "/api/timeline?limit=10", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []*model.Status
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp) != 1 || resp[0].ID != "timeline-1" {
		t.Errorf("unexpected timeline response")
	}

	// Test invalid limit
	reqErr := httptest.NewRequest("GET", "/api/timeline?limit=-5", nil)
	rrErr := httptest.NewRecorder()
	mux.ServeHTTP(rrErr, reqErr)
	if rrErr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid limit, got %d", rrErr.Code)
	}
}

func TestSearchRoute(t *testing.T) {
	buf := buffer.New(10)
	idx := &mockSearchIndex{}
	fc := mastodon.NewFakeClient()
	pipe := ingest.New(fc.Events(), buf, idx)

	mux := api.NewRouter(api.RouterConfig{
		StaticFS: nil,
		Buffer:   buf,
		Index:    idx,
		Pipeline: pipe,
		Client:   fc,
	})

	req := httptest.NewRequest("GET", "/api/search?q=test&limit=5&offset=2", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	if idx.searchedQuery != "test" {
		t.Errorf("expected query 'test', got '%s'", idx.searchedQuery)
	}
	if idx.searchOpts.Limit != 5 || idx.searchOpts.Offset != 2 {
		t.Errorf("unexpected options: %+v", idx.searchOpts)
	}

	var resp index.SearchResult
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode search result: %v", err)
	}

	if len(resp.Hits) != 1 || resp.Hits[0].ID != "match-1" {
		t.Errorf("unexpected search hit")
	}

	// Test missing q
	reqErr := httptest.NewRequest("GET", "/api/search?limit=5", nil)
	rrErr := httptest.NewRecorder()
	mux.ServeHTTP(rrErr, reqErr)
	if rrErr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on missing q, got %d", rrErr.Code)
	}
}

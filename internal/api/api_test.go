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
	mockStats     any
	mockStatsErr  error
	mockURL       string
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

func (m *mockSearchIndex) GetSinceID(ctx context.Context) (string, error) {
	return "", nil
}

func (m *mockSearchIndex) SaveSinceID(ctx context.Context, sinceID string) error {
	return nil
}

func (m *mockSearchIndex) Stats(ctx context.Context) (any, error) {
	return m.mockStats, m.mockStatsErr
}

func (m *mockSearchIndex) URL() string {
	return m.mockURL
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

func TestHealthzRoute(t *testing.T) {
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

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var healthyResp api.HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&healthyResp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}

	if healthyResp.Status != "healthy" || !healthyResp.Initialized || healthyResp.Uptime == "" {
		t.Errorf("unexpected health response payload: %+v", healthyResp)
	}
}

func TestStatuszRoute(t *testing.T) {
	buf := buffer.New(10)
	idx := &mockSearchIndex{
		mockURL: "http://localhost:7700",
		mockStats: map[string]any{
			"databaseSize": int64(1234),
			"indexes": map[string]any{
				"statuses": map[string]any{
					"numberOfDocuments": int64(42),
				},
			},
		},
	}
	fc := mastodon.NewFakeClient()
	pipe := ingest.New(fc.Events(), buf, idx)

	mux := api.NewRouter(api.RouterConfig{
		StaticFS: nil,
		Buffer:   buf,
		Index:    idx,
		Pipeline: pipe,
		Client:   fc,
	})

	_ = fc.Connect(context.Background())

	req := httptest.NewRequest("GET", "/statusz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp api.StatuszResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode statusz response: %v", err)
	}

		// Verify Mastodon status
	mast := resp.Dependencies.Mastodon
	if !mast.Connected || mast.Server != "fake-server" || mast.Stream != "fake-stream" {
		t.Errorf("unexpected mastodon status: %+v", mast)
	}

	// Verify MeiliSearch status
	m := resp.Dependencies.MeiliSearch
	if !m.Enabled || !m.Connected || m.URL != "http://localhost:7700" || m.Error != "" {
		t.Errorf("unexpected meilisearch status: %+v", m)
	}
	statsMap, ok := m.Stats.(map[string]any)
	if !ok {
		t.Fatalf("expected stats to be a map, got %T", m.Stats)
	}
	if statsMap["databaseSize"] != float64(1234) { // json unmarshals numbers to float64 by default
		t.Errorf("expected databaseSize 1234, got %v", statsMap["databaseSize"])
	}

	// 2. Verify disabled MeiliSearch status
	idxDisabled := &mockSearchIndex{
		mockURL: "",
	}
	muxDisabled := api.NewRouter(api.RouterConfig{
		StaticFS: nil,
		Buffer:   buf,
		Index:    idxDisabled,
		Pipeline: pipe,
		Client:   fc,
	})

	rr2 := httptest.NewRecorder()
	muxDisabled.ServeHTTP(rr2, req)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}

	var respDisabled api.StatuszResponse
	if err := json.NewDecoder(rr2.Body).Decode(&respDisabled); err != nil {
		t.Fatalf("failed to decode disabled statusz response: %v", err)
	}

	mDisabled := respDisabled.Dependencies.MeiliSearch
	if mDisabled.Enabled || mDisabled.Connected || mDisabled.URL != "" || mDisabled.Stats != nil {
		t.Errorf("expected disabled meilisearch status, got: %+v", mDisabled)
	}
}

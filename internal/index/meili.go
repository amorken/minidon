package index

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/meilisearch/meilisearch-go"

	"github.com/amorken/minidon/internal/model"
)

type meiliIndex struct {
	url       string
	masterKey string

	mu       sync.Mutex
	client   meilisearch.ServiceManager
	index    meilisearch.IndexManager
	resolved bool
}

// NewMeiliIndex constructs a new MeiliSearch Index implementation.
func NewMeiliIndex(url, apiKey string) Index {
	return &meiliIndex{
		url:       url,
		masterKey: apiKey,
	}
}

func (m *meiliIndex) initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.resolved {
		return nil
	}

	hc := &http.Client{
		Timeout: 5 * time.Second,
	}

	// If no master key is configured, run in dev mode without resolving an admin key.
	if m.masterKey == "" {
		slog.Info("meili: no master key provided, using direct client connection")
		client := meilisearch.New(m.url, meilisearch.WithCustomClient(hc))
		m.client = client
		m.index = client.Index("statuses")
		m.resolved = true
		return nil
	}

	slog.Info("meili: resolving Default Admin API Key using master key")
	masterClient := meilisearch.New(m.url, meilisearch.WithAPIKey(m.masterKey), meilisearch.WithCustomClient(hc))

	keysResults, err := masterClient.GetKeysWithContext(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to retrieve keys using master key: %w", err)
	}

	var adminKey string
	for _, k := range keysResults.Results {
		if k.Name == "Default Admin API Key" {
			adminKey = k.Key
			break
		}
	}

	if adminKey == "" {
		return fmt.Errorf("Default Admin API Key not found in Meilisearch response")
	}

	slog.Info("meili: successfully retrieved Default Admin API Key")
	client := meilisearch.New(m.url, meilisearch.WithAPIKey(adminKey), meilisearch.WithCustomClient(hc))
	m.client = client
	m.index = client.Index("statuses")
	m.resolved = true
	return nil
}

// Index queues a batch of statuses to be added to MeiliSearch.
func (m *meiliIndex) Index(statuses []model.Status) error {
	if len(statuses) == 0 {
		return nil
	}
	if err := m.initialize(context.Background()); err != nil {
		return fmt.Errorf("meili: failed to initialize client: %w", err)
	}
	_, err := m.index.AddDocuments(statuses, nil)
	if err != nil {
		return fmt.Errorf("meili: failed to add documents: %w", err)
	}
	return nil
}

// Search queries MeiliSearch for statuses matching the query and search options.
func (m *meiliIndex) Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error) {
	if err := m.initialize(ctx); err != nil {
		return SearchResult{}, fmt.Errorf("meili: failed to initialize client: %w", err)
	}
	req := &meilisearch.SearchRequest{
		Limit:  int64(opts.Limit),
		Offset: int64(opts.Offset),
	}
	resp, err := m.index.SearchWithContext(ctx, query, req)
	if err != nil {
		return SearchResult{}, fmt.Errorf("meili: search failed: %w", err)
	}

	hitsJSON, err := json.Marshal(resp.Hits)
	if err != nil {
		return SearchResult{}, fmt.Errorf("meili: failed to marshal hits: %w", err)
	}

	hits := []model.Status{}
	if err := json.Unmarshal(hitsJSON, &hits); err != nil {
		return SearchResult{}, fmt.Errorf("meili: failed to unmarshal hits to statuses: %w", err)
	}

	return SearchResult{
		Hits:   hits,
		Total:  resp.EstimatedTotalHits,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}

func (m *meiliIndex) applySettings(ctx context.Context) error {
	if err := m.initialize(ctx); err != nil {
		return fmt.Errorf("meili: failed to initialize client: %w", err)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	searchable := []string{"content", "account.acct", "account.display_name", "tags.name"}
	if _, err := m.index.UpdateSearchableAttributesWithContext(attemptCtx, &searchable); err != nil {
		return fmt.Errorf("meili: failed to update searchable attributes: %w", err)
	}

	sortable := []string{"created_at"}
	if _, err := m.index.UpdateSortableAttributesWithContext(attemptCtx, &sortable); err != nil {
		return fmt.Errorf("meili: failed to update sortable attributes: %w", err)
	}

	filterable := []any{"language", "tags.name"}
	if _, err := m.index.UpdateFilterableAttributesWithContext(attemptCtx, &filterable); err != nil {
		return fmt.Errorf("meili: failed to update filterable attributes: %w", err)
	}

	return nil
}

// EnsureSettings applies searchable, sortable, and filterable index configuration idempotently.
// If MeiliSearch is starting up, it retries with a backoff.
func (m *meiliIndex) EnsureSettings(ctx context.Context) error {
	var err error
	for i := 0; i < 5; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err = m.applySettings(ctx)
		if err == nil {
			slog.Info("meili: settings applied successfully")
			return nil
		}

		slog.Warn("meili: failed to apply settings, retrying...", "err", err)
		m.sleep(ctx, 2*time.Second)
	}

	return fmt.Errorf("meili: failed to apply settings after retries: %w", err)
}

func (m *meiliIndex) sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

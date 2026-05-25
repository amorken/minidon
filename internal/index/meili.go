package index

import (
	"context"
	"encoding/json"
	"errors"
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

// Delete removes a status from MeiliSearch index by ID.
func (m *meiliIndex) Delete(ctx context.Context, id string) error {
	if err := m.initialize(ctx); err != nil {
		return fmt.Errorf("meili: failed to initialize client: %w", err)
	}
	_, err := m.index.DeleteDocumentWithContext(ctx, id, nil)
	if err != nil {
		return fmt.Errorf("meili: failed to delete document: %w", err)
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
	for range 5 {
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

type SinceIDState struct {
	ID      string `json:"id"`
	SinceID string `json:"since_id"`
}

func (m *meiliIndex) GetSinceID(ctx context.Context) (string, error) {
	if err := m.initialize(ctx); err != nil {
		return "", fmt.Errorf("meili: failed to initialize client: %w", err)
	}

	var state SinceIDState
	err := m.client.Index("minidon_state").GetDocumentWithContext(ctx, "pagination", nil, &state)
	if err != nil {
		var meiliErr *meilisearch.Error
		if errors.As(err, &meiliErr) {
			if meiliErr.StatusCode == http.StatusNotFound || meiliErr.MeilisearchApiError.Code == "document_not_found" || meiliErr.MeilisearchApiError.Code == "index_not_found" {
				slog.Debug("meili: no existing since_id found in index")
				return "", nil
			}
		}
		return "", fmt.Errorf("meili: failed to get since_id state: %w", err)
	}
	slog.Debug("meili: retrieved since_id from index", "since_id", state.SinceID)
	return state.SinceID, nil
}

func (m *meiliIndex) SaveSinceID(ctx context.Context, sinceID string) error {
	if err := m.initialize(ctx); err != nil {
		return fmt.Errorf("meili: failed to initialize client: %w", err)
	}

	slog.Debug("meili: saving since_id to index", "since_id", sinceID)
	state := []SinceIDState{
		{
			ID:      "pagination",
			SinceID: sinceID,
		},
	}

	primaryKey := "id"
	_, err := m.client.Index("minidon_state").AddDocumentsWithContext(ctx, state, &meilisearch.DocumentOptions{
		PrimaryKey: &primaryKey,
	})
	if err != nil {
		return fmt.Errorf("meili: failed to save since_id state: %w", err)
	}
	return nil
}

// Clear removes all documents from the MeiliSearch "statuses" and "minidon_state" indices.
// If the indices do not exist, the error is handled gracefully.
func Clear(ctx context.Context, url, apiKey string) error {
	m := &meiliIndex{
		url:       url,
		masterKey: apiKey,
	}
	if err := m.initialize(ctx); err != nil {
		return fmt.Errorf("meili: failed to initialize client: %w", err)
	}

	slog.Info("meili: clearing all documents from statuses index")
	_, err := m.index.DeleteAllDocumentsWithContext(ctx, nil)
	if err != nil {
		var meiliErr *meilisearch.Error
		if errors.As(err, &meiliErr) && (meiliErr.StatusCode == http.StatusNotFound || meiliErr.MeilisearchApiError.Code == "index_not_found") {
			slog.Debug("meili: statuses index not found, skipping document deletion")
		} else {
			return fmt.Errorf("meili: failed to clear statuses documents: %w", err)
		}
	}

	slog.Info("meili: clearing pagination state from minidon_state index")
	_, err = m.client.Index("minidon_state").DeleteAllDocumentsWithContext(ctx, nil)
	if err != nil {
		var meiliErr *meilisearch.Error
		if errors.As(err, &meiliErr) && (meiliErr.StatusCode == http.StatusNotFound || meiliErr.MeilisearchApiError.Code == "index_not_found") {
			slog.Debug("meili: minidon_state index not found, skipping pagination deletion")
		} else {
			return fmt.Errorf("meili: failed to clear pagination state: %w", err)
		}
	}

	return nil
}



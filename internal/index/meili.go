package index

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/meilisearch/meilisearch-go"

	"github.com/amorken/minidon/internal/model"
)

type meiliIndex struct {
	client meilisearch.ServiceManager
	index  meilisearch.IndexManager
}

// NewMeiliIndex constructs a new MeiliSearch Index implementation.
func NewMeiliIndex(url, apiKey string) Index {
	hc := &http.Client{
		Timeout: 5 * time.Second,
	}
	client := meilisearch.New(url, meilisearch.WithAPIKey(apiKey), meilisearch.WithCustomClient(hc))
	return &meiliIndex{
		client: client,
		index:  client.Index("statuses"),
	}
}

// Index queues a batch of statuses to be added to MeiliSearch.
func (m *meiliIndex) Index(statuses []model.Status) error {
	if len(statuses) == 0 {
		return nil
	}
	_, err := m.index.AddDocuments(statuses, nil)
	if err != nil {
		return fmt.Errorf("meili: failed to add documents: %w", err)
	}
	return nil
}

// Search queries MeiliSearch for statuses matching the query and search options.
func (m *meiliIndex) Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error) {
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

	var hits []model.Status
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

		// Create a separate child context with timeout for settings update calls to avoid blocking indefinitely when MeiliSearch is offline
		attemptCtx, cancel := context.WithTimeout(ctx, 3*time.Second)

		searchable := []string{"content", "account.acct", "account.display_name", "tags.name"}
		_, err = m.index.UpdateSearchableAttributesWithContext(attemptCtx, &searchable)
		if err != nil {
			cancel()
			slog.Warn("meili: failed to update searchable attributes, retrying...", "err", err)
			m.sleep(ctx, 2*time.Second)
			continue
		}

		sortable := []string{"created_at"}
		_, err = m.index.UpdateSortableAttributesWithContext(attemptCtx, &sortable)
		if err != nil {
			cancel()
			slog.Warn("meili: failed to update sortable attributes, retrying...", "err", err)
			m.sleep(ctx, 2*time.Second)
			continue
		}

		filterable := []any{"language", "tags.name"}
		_, err = m.index.UpdateFilterableAttributesWithContext(attemptCtx, &filterable)
		cancel()
		if err != nil {
			slog.Warn("meili: failed to update filterable attributes, retrying...", "err", err)
			m.sleep(ctx, 2*time.Second)
			continue
		}

		slog.Info("meili: settings applied successfully")
		return nil
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

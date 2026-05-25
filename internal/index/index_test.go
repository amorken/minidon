package index_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/model"
)

func TestNoopIndex(t *testing.T) {
	idx := &index.NoopIndex{}

	statuses := []model.Status{
		{ID: "1", Content: "hello"},
	}

	err := idx.Index(statuses)
	if err != nil {
		t.Fatalf("unexpected error for NoopIndex.Index: %v", err)
	}

	res, err := idx.Search(context.Background(), "hello", index.SearchOptions{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error for NoopIndex.Search: %v", err)
	}

	if len(res.Hits) != 0 {
		t.Errorf("expected 0 hits from NoopIndex, got %d", len(res.Hits))
	}
	if res.Total != 0 {
		t.Errorf("expected 0 total hits, got %d", res.Total)
	}
	if res.Limit != 10 {
		t.Errorf("expected limit 10, got %d", res.Limit)
	}
}

func TestMeiliIndex_Initialize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer master-key-123" && auth != "Bearer admin-key-xyz" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/keys" && r.Method == "GET" {
			if auth != "Bearer master-key-123" {
				http.Error(w, "Forbidden - keys endpoint requires master key", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results": [
					{
						"name": "Default Search API Key",
						"key": "search-key-abc"
					},
					{
						"name": "Default Admin API Key",
						"key": "admin-key-xyz"
					}
				]
			}`))
			return
		}

		// Subsequent operations should use the admin-key-xyz
		if r.URL.Path == "/indexes/statuses/settings" {
			if auth != "Bearer admin-key-xyz" {
				http.Error(w, "Forbidden - settings endpoint requires admin API key", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid": 1}`))
			return
		}

		// UpdateSearchableAttributes, UpdateSortableAttributes, UpdateFilterableAttributes
		// all map to "/indexes/statuses/settings/searchable", "/indexes/statuses/settings/sortable", etc.
		if r.Method == "PUT" || r.Method == "POST" || r.Method == "DELETE" {
			if auth != "Bearer admin-key-xyz" {
				http.Error(w, "Forbidden - modifying operations require admin API key", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid": 1}`))
			return
		}

		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	idx := index.NewMeiliIndex(server.URL, "master-key-123")

	err := idx.EnsureSettings(context.Background())
	if err != nil {
		t.Fatalf("EnsureSettings failed: %v", err)
	}
}

func TestMeiliIndex_SinceID(t *testing.T) {
	var savedPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer master-key-123" && auth != "Bearer admin-key-xyz" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/keys" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results": [
					{
						"name": "Default Admin API Key",
						"key": "admin-key-xyz"
					}
				]
			}`))
			return
		}

		if r.URL.Path == "/indexes/minidon_state/documents" && r.Method == "POST" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			savedPayload = body
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid": 1}`))
			return
		}

		if r.URL.Path == "/indexes/minidon_state/documents/pagination" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			if savedPayload == nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"Document not found","code":"document_not_found","type":"invalid_request","link":"https://docs.meilisearch.com/errors#document_not_found"}`))
				return
			}
			var list []map[string]interface{}
			if err := json.Unmarshal(savedPayload, &list); err != nil || len(list) == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			docBytes, _ := json.Marshal(list[0])
			_, _ = w.Write(docBytes)
			return
		}

		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	idx := index.NewMeiliIndex(server.URL, "master-key-123")

	val, err := idx.GetSinceID(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on initial GetSinceID: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty since_id, got %q", val)
	}

	err = idx.SaveSinceID(context.Background(), "987654321")
	if err != nil {
		t.Fatalf("unexpected error on SaveSinceID: %v", err)
	}

	val, err = idx.GetSinceID(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on GetSinceID after save: %v", err)
	}
	if val != "987654321" {
		t.Errorf("expected since_id '987654321', got %q", val)
	}
}

func TestMeiliIndex_Clear(t *testing.T) {
	var statusesDeleted, stateDeleted bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer master-key-123" && auth != "Bearer admin-key-xyz" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/keys" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results": [
					{
						"name": "Default Admin API Key",
						"key": "admin-key-xyz"
					}
				]
			}`))
			return
		}

		if r.URL.Path == "/indexes/statuses/documents" && r.Method == "DELETE" {
			statusesDeleted = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid": 1}`))
			return
		}

		if r.URL.Path == "/indexes/minidon_state/documents" && r.Method == "DELETE" {
			stateDeleted = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid": 2}`))
			return
		}

		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	err := index.Clear(context.Background(), server.URL, "master-key-123")
	if err != nil {
		t.Fatalf("unexpected error on Clear: %v", err)
	}

	if !statusesDeleted {
		t.Error("expected statuses documents deletion request, but none was received")
	}
	if !stateDeleted {
		t.Error("expected minidon_state documents deletion request, but none was received")
	}
}

func TestMeiliIndex_Clear_IndexNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer master-key-123" && auth != "Bearer admin-key-xyz" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/keys" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results": [
					{
						"name": "Default Admin API Key",
						"key": "admin-key-xyz"
					}
				]
			}`))
			return
		}

		if r.URL.Path == "/indexes/statuses/documents" && r.Method == "DELETE" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Index not found","code":"index_not_found","type":"invalid_request"}`))
			return
		}

		if r.URL.Path == "/indexes/minidon_state/documents" && r.Method == "DELETE" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Index not found","code":"index_not_found","type":"invalid_request"}`))
			return
		}

		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	err := index.Clear(context.Background(), server.URL, "master-key-123")
	if err != nil {
		t.Fatalf("expected no error when index is not found, got: %v", err)
	}
}




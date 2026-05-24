package static_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/amorken/minidon/internal/static"
)

func TestStaticHandler_ServesExistingFile(t *testing.T) {
	mockFS := fstest.MapFS{
		"web/dist/index.html": &fstest.MapFile{
			Data: []byte("minidon main page"),
		},
	}
	handler := static.NewHandler(mockFS)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if !strings.Contains(string(body), "minidon") {
		t.Errorf("expected body to contain 'minidon', got: %s", string(body))
	}
}

func TestStaticHandler_SPAFallback(t *testing.T) {
	mockFS := fstest.MapFS{
		"web/dist/index.html": &fstest.MapFile{
			Data: []byte("minidon fallback"),
		},
	}
	handler := static.NewHandler(mockFS)
	req := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if !strings.Contains(string(body), "minidon") {
		t.Errorf("expected body to fall back to 'minidon', got: %s", string(body))
	}
}

func TestStaticHandler_AssetsCacheControl(t *testing.T) {
	mockFS := fstest.MapFS{
		"web/dist/assets/index-zJePbD5o.js": &fstest.MapFile{
			Data: []byte("console.log('test')"),
		},
	}
	handler := static.NewHandler(mockFS)
	req := httptest.NewRequest(http.MethodGet, "/assets/index-zJePbD5o.js", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	cacheControl := rr.Header().Get("Cache-Control")
	expected := "public, max-age=31536000, immutable"
	if cacheControl != expected {
		t.Errorf("expected Cache-Control %q, got %q", expected, cacheControl)
	}
}

func TestStaticHandler_IndexHTMLCacheControl(t *testing.T) {
	mockFS := fstest.MapFS{
		"web/dist/index.html": &fstest.MapFile{
			Data: []byte("minidon index"),
		},
	}
	handler := static.NewHandler(mockFS)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	cacheControl := rr.Header().Get("Cache-Control")
	expected := "no-cache"
	if cacheControl != expected {
		t.Errorf("expected Cache-Control %q, got %q", expected, cacheControl)
	}
}

func TestStaticHandler_DefaultCacheControl(t *testing.T) {
	mockFS := fstest.MapFS{
		"web/dist/robots.txt": &fstest.MapFile{
			Data: []byte("User-agent: *"),
		},
	}
	handler := static.NewHandler(mockFS)
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	cacheControl := rr.Header().Get("Cache-Control")
	expected := "public, max-age=300"
	if cacheControl != expected {
		t.Errorf("expected Cache-Control %q, got %q", expected, cacheControl)
	}
}

func TestStaticHandler_BlocksAPIPaths(t *testing.T) {
	mockFS := fstest.MapFS{}
	handler := static.NewHandler(mockFS)
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestStaticHandler_MethodNotAllowed(t *testing.T) {
	mockFS := fstest.MapFS{}
	handler := static.NewHandler(mockFS)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

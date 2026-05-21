package static_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amorken/minidon"
	"github.com/amorken/minidon/internal/static"
)

func TestStaticHandler_ServesExistingFile(t *testing.T) {
	handler := static.NewHandler(minidon.StaticFS)
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
	handler := static.NewHandler(minidon.StaticFS)
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
	handler := static.NewHandler(minidon.StaticFS)
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
	handler := static.NewHandler(minidon.StaticFS)
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
	handler := static.NewHandler(minidon.StaticFS)
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
	handler := static.NewHandler(minidon.StaticFS)
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestStaticHandler_MethodNotAllowed(t *testing.T) {
	handler := static.NewHandler(minidon.StaticFS)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOllamaEmbedderUsesV2Endpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3]]}`))
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text", 2*time.Second)
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(vec))
	}
}

func TestOllamaEmbedderFallsBackToLegacy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		case "/api/embeddings":
			_, _ = w.Write([]byte(`{"embedding":[0.4,0.5]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text", 2*time.Second)
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(vec) != 2 {
		t.Fatalf("expected 2 dims, got %d", len(vec))
	}
}

package search

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOllamaEmbedderUsesV2Endpoint(t *testing.T) {
	e := NewOllamaEmbedder("http://ollama.test", "nomic-embed-text", 2*time.Second)
	e.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"embeddings":[[0.1,0.2,0.3]]}`), nil
	})}
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(vec))
	}
}

func TestOllamaEmbedderFallsBackToLegacy(t *testing.T) {
	e := NewOllamaEmbedder("http://ollama.test", "nomic-embed-text", 2*time.Second)
	e.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/embed":
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		case "/api/embeddings":
			return jsonResponse(http.StatusOK, `{"embedding":[0.4,0.5]}`), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
			return nil, nil
		}
		return nil, nil
	})}
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(vec) != 2 {
		t.Fatalf("expected 2 dims, got %d", len(vec))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

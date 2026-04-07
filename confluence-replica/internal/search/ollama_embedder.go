package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaEmbedder(baseURL, model string, timeout time.Duration) *OllamaEmbedder {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &OllamaEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	if vec, err := e.embedV2(ctx, text); err == nil && len(vec) > 0 {
		return vec, nil
	}
	return e.embedLegacy(ctx, text)
}

func (e *OllamaEmbedder) embedV2(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]any{"model": e.model, "input": text}
	raw, err := e.postJSON(ctx, "/api/embed", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Embeddings [][]float32 `json:"embeddings"`
		Embedding  []float32   `json:"embedding"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if len(resp.Embeddings) > 0 {
		return resp.Embeddings[0], nil
	}
	if len(resp.Embedding) > 0 {
		return resp.Embedding, nil
	}
	return nil, fmt.Errorf("empty embedding from /api/embed")
}

func (e *OllamaEmbedder) embedLegacy(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]any{"model": e.model, "prompt": text}
	raw, err := e.postJSON(ctx, "/api/embeddings", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if len(resp.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding from /api/embeddings")
	}
	return resp.Embedding, nil
}

func (e *OllamaEmbedder) postJSON(ctx context.Context, p string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+p, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama %s status %d: %s", p, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

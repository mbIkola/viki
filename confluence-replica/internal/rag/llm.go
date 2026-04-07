package rag

import (
	"context"
	"fmt"
	"strings"
)

type EchoLLM struct{}

func (e EchoLLM) Complete(_ context.Context, query string, ctxHits []SearchHit) (string, error) {
	if len(ctxHits) == 0 {
		return "", nil
	}
	top := ctxHits[0]
	return fmt.Sprintf("Answer draft for %q based on local index snippet: %s", query, strings.TrimSpace(top.Snippet)), nil
}

type DeterministicLLM struct{}

func (d DeterministicLLM) Complete(_ context.Context, query string, ctxHits []SearchHit) (string, error) {
	if len(ctxHits) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Local synthesis for %q.\n", query))
	b.WriteString("Only indexed snippets were used:\n")
	for i, h := range ctxHits {
		snippet := strings.TrimSpace(h.Snippet)
		if snippet == "" {
			continue
		}
		if len(snippet) > 280 {
			snippet = snippet[:280]
		}
		b.WriteString(fmt.Sprintf("%d. [%s | page=%s v=%d] %s\n", i+1, h.Title, h.PageID, h.Version, snippet))
	}
	return strings.TrimSpace(b.String()), nil
}

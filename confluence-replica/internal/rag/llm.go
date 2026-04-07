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

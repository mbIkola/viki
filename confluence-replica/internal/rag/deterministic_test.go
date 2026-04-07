package rag

import (
	"context"
	"strings"
	"testing"
)

type lowScoreRetriever struct{}

func (l lowScoreRetriever) Retrieve(_ context.Context, _ string, _ int) ([]SearchHit, error) {
	return []SearchHit{
		{PageID: "1", Version: 1, Title: "Low", Snippet: "thin context", Score: 0.01},
	}, nil
}

func TestChatReturnsRefusalOnWeakContext(t *testing.T) {
	engine := NewEngine(lowScoreRetriever{}, DeterministicLLM{})
	resp, err := engine.Answer(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Refused {
		t.Fatalf("expected refusal on weak context")
	}
}

func TestDeterministicLLMOnlyUsesRetrievedSnippets(t *testing.T) {
	llm := DeterministicLLM{}
	out, err := llm.Complete(context.Background(), "what changed", []SearchHit{
		{PageID: "42", Version: 7, Title: "Runbook", Snippet: "critical path changed yesterday", Score: 0.9},
		{PageID: "43", Version: 2, Title: "Guide", Snippet: "rollout steps updated", Score: 0.7},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "critical path changed yesterday") {
		t.Fatalf("expected top snippet in output: %s", out)
	}
	if !strings.Contains(out, "rollout steps updated") {
		t.Fatalf("expected secondary snippet in output: %s", out)
	}
}

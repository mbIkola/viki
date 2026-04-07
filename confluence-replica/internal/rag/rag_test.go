package rag

import (
	"context"
	"testing"
)

type fakeRetriever struct{}

func (f fakeRetriever) Retrieve(_ context.Context, _ string, _ int) ([]SearchHit, error) {
	return []SearchHit{
		{PageID: "42", Version: 7, Title: "Runbook", Snippet: "critical path", Score: 0.9},
	}, nil
}

type fakeLLM struct{}

func (f fakeLLM) Complete(_ context.Context, _ string, _ []SearchHit) (string, error) {
	return "Answer based on local replica", nil
}

func TestChatResponseContainsCitations(t *testing.T) {
	engine := NewEngine(fakeRetriever{}, fakeLLM{})
	resp, err := engine.Answer(context.Background(), "what changed?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Citations) == 0 {
		t.Fatalf("expected citations")
	}
	if resp.Citations[0].PageID == "" || resp.Citations[0].Version == 0 {
		t.Fatalf("expected citation with page_id and version")
	}
}

func TestChatReturnsRefusalWithoutContext(t *testing.T) {
	engine := NewEngine(emptyRetriever{}, fakeLLM{})
	resp, err := engine.Answer(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Refused {
		t.Fatalf("expected refusal when no local context")
	}
}

type emptyRetriever struct{}

func (e emptyRetriever) Retrieve(_ context.Context, _ string, _ int) ([]SearchHit, error) {
	return nil, nil
}
